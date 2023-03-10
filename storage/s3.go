package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

func NewS3(bucketName string) (Provider, error) {
	var configs []func(*config.LoadOptions) error

	// Used by the test suite
	if val, ok := os.LookupEnv("COCOV_CACHE_S3_ENDPOINT"); ok {
		configs = append(configs, config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               val,
				HostnameImmutable: true,
				PartitionID:       "aws",
			}, nil
		})))
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), configs...)
	if err != nil {
		return nil, err
	}
	s3Client := s3.NewFromConfig(cfg)

	_, err = s3Client.HeadBucket(context.Background(), &s3.HeadBucketInput{
		Bucket: &bucketName,
	})
	if err != nil {
		return nil, err
	}

	l := s3Provider{
		bucketName: aws.String(bucketName),
		client:     s3Client,
		config:     cfg,
	}
	return l, nil
}

type s3Provider struct {
	bucketName *string
	client     *s3.Client
	config     aws.Config
}

func (s3Provider) prefixFor(k Kind) string {
	if k == KindArtifact {
		return "artifact"
	}

	return "tool"
}

func (s s3Provider) keyFor(locator ObjectDescriptor) string {
	return s.prefixFor(locator.kind()) + "/" + strings.Join(locator.PathComponents(), "/")
}

func coerceAWSError(key string, err error) error {
	var (
		bne *types.NoSuchBucket
		nsk *types.NoSuchKey
	)
	if errors.As(err, &bne) || errors.As(err, &nsk) {
		return ErrNotExist{path: key}
	}

	if sErr, ok := err.(*smithy.OperationError); ok {
		errString := sErr.Error()
		if strings.Contains(errString, "NoSuchBucket") || strings.Contains(errString, "NoSuchKey") {
			return ErrNotExist{path: key}
		}
	}

	return err
}

var typeOfTime = reflect.TypeOf(time.Time{})

func applyS3Tags(tags []types.Tag, into *Item) error {
	fieldTag := map[string]reflect.StructField{}
	var wantedTags []string

	t := reflect.TypeOf(Item{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag, ok := f.Tag.Lookup("tag")
		if ok {
			wantedTags = append(wantedTags, tag)
			fieldTag[tag] = f
		}
	}

	val := reflect.ValueOf(into)
	val = val.Elem()
	for _, tagName := range wantedTags {
		field := fieldTag[tagName]
		set := false
		for _, tagPair := range tags {
			if tagPair.Key == nil || tagPair.Value == nil {
				continue
			}

			if tagPair.Key != nil && *tagPair.Key == tagName {

				if field.Type.Kind() == reflect.String {
					val.FieldByIndex(field.Index).SetString(*tagPair.Value)
					set = true
					break
				} else if field.Type.Kind() == reflect.Int {
					i, err := strconv.Atoi(*tagPair.Value)
					if err != nil {
						return fmt.Errorf("parsing value %s for field %s: %w", *tagPair.Value, field.Name, err)
					}
					val.FieldByIndex(field.Index).SetInt(int64(i))
					set = true
					break
				} else if field.Type == typeOfTime {
					t, err := time.Parse(time.RFC3339, *tagPair.Value)
					if err != nil {
						return fmt.Errorf("parsing value %s for field %s: %w", *tagPair.Value, field.Name, err)
					}
					val.FieldByIndex(field.Index).Set(reflect.ValueOf(t))
					set = true
					break
				} else {
					return fmt.Errorf("field %s contains unsupported type %s", field.Name, field.Type)
				}
			}
		}

		if !set {
			return fmt.Errorf("could not obtain value for field %s (%s)", field.Name, tagName)
		}
	}

	return nil
}

func (s s3Provider) MetadataOf(locator ObjectDescriptor) (*Item, error) {
	key := s.keyFor(locator)
	tags, err := s.client.GetObjectTagging(context.Background(), &s3.GetObjectTaggingInput{
		Bucket: s.bucketName,
		Key:    &key,
	})
	if err != nil {
		return nil, coerceAWSError(key, err)
	}

	i := &Item{}
	if err = applyS3Tags(tags.TagSet, i); err != nil {
		return nil, err
	}

	return i, nil
}

func (s s3Provider) Get(locator ObjectDescriptor) (*Item, io.ReadCloser, error) {
	key := s.keyFor(locator)
	meta, err := s.MetadataOf(locator)
	if err != nil {
		return nil, nil, err
	}

	obj, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: s.bucketName,
		Key:    &key,
	})
	if err != nil {
		return nil, nil, coerceAWSError(key, err)
	}

	return meta, obj.Body, nil
}

func tagsFromItem(item *Item) map[string][]string {
	vals := map[string][]string{}

	t := reflect.TypeOf(item).Elem()
	v := reflect.ValueOf(item).Elem()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag, ok := f.Tag.Lookup("tag")
		if !ok {
			continue
		}
		val := v.FieldByIndex(f.Index)
		if val.IsZero() {
			continue
		}

		switch {
		case f.Type.Kind() == reflect.Int:
			vals[tag] = []string{strconv.Itoa(int(v.FieldByIndex(f.Index).Int()))}
		case f.Type.Kind() == reflect.String:
			vals[tag] = []string{v.FieldByIndex(f.Index).String()}
		case f.Type == typeOfTime:
			vals[tag] = []string{v.FieldByIndex(f.Index).Interface().(time.Time).Format(time.RFC3339)}
		default:
			panic(fmt.Errorf("unexpected type %s on tag %s", f.Type, tag))
		}
	}

	return vals
}

func awsTagsFromTags(tags map[string][]string) []types.Tag {
	res := make([]types.Tag, 0, len(tags))
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		(func(k string) {
			res = append(res, types.Tag{Key: &k, Value: &tags[k][0]})
		})(k)
	}
	return res
}

func (s s3Provider) Set(locator ObjectDescriptor, mime string, objectSize int, stream io.ReadCloser) error {
	defer func() { _ = stream.Close() }()

	key := s.keyFor(locator)
	item := Item{
		CreatedAt:  time.Now().UTC(),
		AccessedAt: time.Now().UTC(),
		Size:       objectSize,
		Mime:       mime,
	}

	objectTags := url.Values(tagsFromItem(&item)).Encode()
	uploader := manager.NewUploader(s.client)
	_, err := uploader.Upload(context.Background(), &s3.PutObjectInput{
		Bucket:  s.bucketName,
		Key:     &key,
		Body:    stream,
		Tagging: &objectTags,
	})

	return err
}

func (s s3Provider) Touch(locator ObjectDescriptor) error {
	key := s.keyFor(locator)
	meta, err := s.MetadataOf(locator)
	if err != nil {
		return err
	}
	meta.AccessedAt = time.Now().UTC()
	tags := awsTagsFromTags(tagsFromItem(meta))

	_, err = s.client.PutObjectTagging(context.Background(), &s3.PutObjectTaggingInput{
		Bucket:  s.bucketName,
		Key:     &key,
		Tagging: &types.Tagging{TagSet: tags},
	})
	return coerceAWSError(key, err)
}

func (s s3Provider) Delete(locator ObjectDescriptor) error {
	k := s.keyFor(locator)
	_, err := s.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: s.bucketName,
		Key:    &k,
	})
	return coerceAWSError(k, err)
}

func (s s3Provider) listObjectsRecursive(req *s3.ListObjectsV2Input, output *s3.ListObjectsV2Output) ([]types.Object, error) {
	objects := output.Contents
	if output.IsTruncated {
		req.ContinuationToken = output.NextContinuationToken
		extraResponse, err := s.client.ListObjectsV2(context.Background(), req)
		if err != nil {
			return nil, err
		}
		extra, err := s.listObjectsRecursive(req, extraResponse)
		if err != nil {
			return nil, err
		}
		objects = append(objects, extra...)
	}

	return objects, nil
}

func groupsOf[T any](size int, input []T) [][]T {
	if size <= 0 {
		panic("groupsOf with negative size")
	}

	i := 0
	inputSize := len(input)
	expectedGroups := int(math.Ceil(float64(len(input)) / float64(size)))
	groups := make([][]T, 0, expectedGroups)
	for i < inputSize {
		upTo := int(math.Min(float64(inputSize), float64(i+size)))
		groups = append(groups, input[i:upTo])
		i += size
	}

	return groups
}

func (s s3Provider) DeleteGroup(locator ObjectDescriptor) error {
	k := s.keyFor(locator)
	req := &s3.ListObjectsV2Input{
		Bucket: s.bucketName,
		Prefix: &k,
	}
	list, err := s.client.ListObjectsV2(context.Background(), req)
	if err != nil {
		return coerceAWSError(k, err)
	}

	objects, err := s.listObjectsRecursive(req, list)
	if err != nil {
		return coerceAWSError(k, err)
	}

	// May only delete 1k items per request...
	requests := groupsOf(1000, objects)
	for _, group := range requests {
		keys := make([]types.ObjectIdentifier, len(group))
		for i, o := range group {
			keys[i].Key = o.Key
		}

		_, err = s.client.DeleteObjects(context.Background(), &s3.DeleteObjectsInput{
			Bucket: s.bucketName,
			Delete: &types.Delete{
				Objects: keys,
			},
		})
		if err != nil {
			return coerceAWSError(k, err)
		}
	}

	return nil
}

func (s s3Provider) TotalSize(kind Kind) (int64, error) {
	prefix := s.prefixFor(kind)
	req := &s3.ListObjectsV2Input{
		Bucket: s.bucketName,
		Prefix: &prefix,
	}
	list, err := s.client.ListObjectsV2(context.Background(), req)
	if err != nil {
		return 0, coerceAWSError(prefix, err)
	}

	objects, err := s.listObjectsRecursive(req, list)
	if err != nil {
		return 0, coerceAWSError(prefix, err)
	}

	size := int64(0)
	for _, v := range objects {
		size += v.Size
	}

	return size, nil
}
