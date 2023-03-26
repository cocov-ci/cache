package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/cocov-ci/cache/api"
	"github.com/cocov-ci/cache/locator"
	"go.uber.org/zap"
	"io"
	"math"
	"os"
	"strings"
)

func NewS3(bucketName string, client api.Client) (Provider, error) {
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
		api:        client,
		bucketName: aws.String(bucketName),
		client:     s3Client,
		config:     cfg,
		log:        zap.L().With(zap.String("component", "S3Storage")),
	}
	return l, nil
}

type s3Provider struct {
	bucketName *string
	client     *s3.Client
	config     aws.Config
	api        api.Client
	log        *zap.Logger
}

func (s s3Provider) artifactKey(repositoryID, artifactKey int64) string {
	return strings.Join([]string{"artifacts", fmt.Sprintf("%d", repositoryID), fmt.Sprintf("%d", artifactKey)}, "/")
}

func (s s3Provider) artifactGroupKey(repositoryID int64) string {
	return strings.Join([]string{"artifacts", fmt.Sprintf("%d", repositoryID), ""}, "/")
}

func (s s3Provider) toolKey(toolID int64) string {
	return strings.Join([]string{"tools", fmt.Sprintf("%d", toolID)}, "/")
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

func (s s3Provider) GetArtifactMeta(locator *locator.ArtifactLocator) (*api.GetArtifactMetaOutput, error) {
	return s.api.GetArtifactMeta(api.GetArtifactMetaInput{
		RepositoryID: locator.RepositoryID,
		NameHash:     locator.NameHash,
		Engine:       "s3",
	})
}

func (s s3Provider) GetArtifact(locator *locator.ArtifactLocator) (*api.GetArtifactMetaOutput, io.ReadCloser, error) {
	meta, err := s.GetArtifactMeta(locator)
	if api.IsNotFound(err) {
		return nil, nil, ErrNotExist{}
	} else if err != nil {
		return nil, nil, err
	}

	objPath := s.artifactKey(locator.RepositoryID, meta.ID)
	obj, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: s.bucketName,
		Key:    &objPath,
	})
	if err != nil {
		s.log.Error("GetObject failed", zap.Error(err), zap.Stringp("bucket", s.bucketName), zap.String("key", objPath))
		return nil, nil, coerceAWSError(objPath, err)
	}

	return meta, obj.Body, nil
}

func (s s3Provider) SetArtifact(locator *locator.ArtifactLocator, mime string, objectSize int64, stream io.ReadCloser) error {
	defer func() { _ = stream.Close() }()

	meta, err := s.api.RegisterArtifact(api.RegisterArtifactInput{
		RepositoryID: locator.RepositoryID,
		Name:         locator.Name,
		NameHash:     locator.NameHash,
		Size:         objectSize,
		Mime:         mime,
		Engine:       "s3",
	})
	if err != nil {

		return err
	}

	deleteArgs := api.DeleteArtifactInput{
		RepositoryID: locator.RepositoryID,
		NameHash:     locator.NameHash,
		Engine:       "s3",
	}

	objPath := s.artifactKey(locator.RepositoryID, meta.ID)
	uploader := manager.NewUploader(s.client)
	_, err = uploader.Upload(context.Background(), &s3.PutObjectInput{
		Bucket: s.bucketName,
		Key:    &objPath,
		Body:   stream,
	})

	if err != nil {
		s.log.Error("Upload failed", zap.Error(err), zap.Stringp("bucket", s.bucketName), zap.String("key", objPath))
		if delErr := s.api.DeleteArtifact(deleteArgs); delErr != nil {
			s.log.Error("Failed removing item from API", zap.Error(err))
		}
		return err
	}

	return nil
}

func (s s3Provider) TouchArtifact(locator *locator.ArtifactLocator) error {
	return s.api.TouchArtifact(api.TouchArtifactInput{
		RepositoryID: locator.RepositoryID,
		NameHash:     locator.NameHash,
		Engine:       "s3",
	})
}

func (s s3Provider) DeleteArtifact(locator *locator.ArtifactLocator) error {
	meta, err := s.GetArtifactMeta(locator)
	if _, ok := err.(ErrNotExist); err != nil && ok {
		return nil
	} else if err != nil {
		return err
	}

	k := s.artifactKey(locator.RepositoryID, meta.ID)
	_, err = s.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: s.bucketName,
		Key:    &k,
	})
	if err != nil {
		return coerceAWSError(k, err)
	}

	return s.api.DeleteArtifact(api.DeleteArtifactInput{
		RepositoryID: locator.RepositoryID,
		NameHash:     locator.NameHash,
		Engine:       "s3",
	})
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

func (s s3Provider) PurgeRepository(id int64) error {
	k := s.artifactGroupKey(id)
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

func (s s3Provider) GetToolMeta(locator *locator.ToolLocator) (*api.GetToolMetaOutput, error) {
	return s.api.GetToolMeta(api.GetToolMetaInput{
		NameHash: locator.NameHash,
		Engine:   "s3",
	})
}

func (s s3Provider) GetTool(locator *locator.ToolLocator) (*api.GetToolMetaOutput, io.ReadCloser, error) {
	meta, err := s.GetToolMeta(locator)
	if api.IsNotFound(err) {
		return nil, nil, ErrNotExist{}
	} else if err != nil {
		return nil, nil, err
	}

	objPath := s.toolKey(meta.ID)
	obj, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: s.bucketName,
		Key:    &objPath,
	})
	if err != nil {
		s.log.Error("GetObject failed", zap.Error(err), zap.Stringp("bucket", s.bucketName), zap.String("key", objPath))
		return nil, nil, coerceAWSError(objPath, err)
	}

	return meta, obj.Body, nil
}

func (s s3Provider) SetTool(locator *locator.ToolLocator, mime string, objectSize int64, stream io.ReadCloser) error {
	defer func() { _ = stream.Close() }()

	meta, err := s.api.RegisterTool(api.RegisterToolInput{
		Name:     locator.Name,
		NameHash: locator.NameHash,
		Size:     objectSize,
		Mime:     mime,
		Engine:   "s3",
	})
	if err != nil {

		return err
	}

	deleteArgs := api.DeleteToolInput{
		NameHash: locator.NameHash,
		Engine:   "s3",
	}

	objPath := s.toolKey(meta.ID)
	uploader := manager.NewUploader(s.client)
	_, err = uploader.Upload(context.Background(), &s3.PutObjectInput{
		Bucket: s.bucketName,
		Key:    &objPath,
		Body:   stream,
	})

	if err != nil {
		s.log.Error("Upload failed", zap.Error(err), zap.Stringp("bucket", s.bucketName), zap.String("key", objPath))
		if delErr := s.api.DeleteTool(deleteArgs); delErr != nil {
			s.log.Error("Failed removing item from API", zap.Error(err))
		}
		return err
	}

	return nil
}

func (s s3Provider) TouchTool(locator *locator.ToolLocator) error {
	return s.api.TouchTool(api.TouchToolInput{
		NameHash: locator.NameHash,
		Engine:   "s3",
	})
}

func (s s3Provider) DeleteTool(locator *locator.ToolLocator) error {
	meta, err := s.GetToolMeta(locator)
	if _, ok := err.(ErrNotExist); err != nil && ok {
		return nil
	} else if err != nil {
		return err
	}

	k := s.toolKey(meta.ID)
	_, err = s.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: s.bucketName,
		Key:    &k,
	})
	if err != nil {
		return coerceAWSError(k, err)
	}

	return s.api.DeleteTool(api.DeleteToolInput{
		NameHash: locator.NameHash,
		Engine:   "s3",
	})
}
