package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyS3Tags(t *testing.T) {
	tag := func(k, v string) types.Tag {
		k = "app.cocov.cache.item." + k
		return types.Tag{Key: &k, Value: &v}
	}
	tags := []types.Tag{
		tag("key", "test.tar.br"),
		tag("created_at", time.Now().UTC().Format(time.RFC3339)),
		tag("accessed_at", time.Now().UTC().Format(time.RFC3339)),
		tag("size", "1048576"),
		tag("mime", "x-application/brotli"),
	}

	item := Item{}
	err := applyS3Tags(tags, &item)
	require.NoError(t, err)
}

func TestTagsFromItem(t *testing.T) {
	t.Run("Complete", func(t *testing.T) {
		tm := time.Date(2009, 11, 17, 20, 34, 58, 651387237, time.UTC)
		item := Item{
			CreatedAt:  tm,
			AccessedAt: tm,
			Size:       1048576,
			Mime:       "x-application/brotli",
		}

		val := url.Values(tagsFromItem(&item)).Encode()
		exp := "app.cocov.cache.item.accessed_at=2009-11-17T20%3A34%3A58Z&app.cocov.cache.item.created_at=2009-11-17T20%3A34%3A58Z&app.cocov.cache.item.mime=x-application%2Fbrotli&app.cocov.cache.item.size=1048576"
		assert.Equal(t, exp, val)
	})

	t.Run("Partial", func(t *testing.T) {
		item := Item{
			Size: 1048576,
		}

		val := url.Values(tagsFromItem(&item)).Encode()
		exp := "app.cocov.cache.item.size=1048576"
		assert.Equal(t, exp, val)
	})
}

func TestGroupsOf(t *testing.T) {
	t.Run("complete", func(t *testing.T) {
		input := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
		groups := groupsOf(3, input)

		assert.Equal(t, groups[0], []int{1, 2, 3})
		assert.Equal(t, groups[1], []int{4, 5, 6})
		assert.Equal(t, groups[2], []int{7, 8, 9})
	})

	t.Run("incomplete", func(t *testing.T) {
		input := []int{1}
		groups := groupsOf(1000, input)
		assert.Len(t, groups, 1)
		assert.Len(t, groups[0], 1)
		assert.Equal(t, 1, groups[0][0])
	})
}

var env = map[string]string{
	"AWS_ACCESS_KEY_ID":       "minioadmin",
	"AWS_SECRET_ACCESS_KEY":   "minioadmin",
	"AWS_REGION":              "us-east-1",
	"COCOV_CACHE_S3_ENDPOINT": "http://localhost:9000",
}

func prepareS3(t *testing.T) (string, *s3.Client) {
	t.Cleanup(func() {
		for k := range env {
			err := os.Unsetenv(k)
			assert.NoError(t, err)
		}
	})
	for k, v := range env {
		err := os.Setenv(k, v)
		require.NoError(t, err)
	}

	bucketName := fmt.Sprintf("cocov-cache-test-%d", time.Now().UTC().UnixMilli())
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
	require.NoError(t, err)

	s3Client := s3.NewFromConfig(cfg)
	_, err = s3Client.CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: &bucketName,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = s3Client.DeleteBucket(context.Background(), &s3.DeleteBucketInput{
			Bucket: &bucketName,
		})
	})

	return bucketName, s3Client
}

func TestS3(t *testing.T) {
	bucketName, s3Client := prepareS3(t)
	var client Provider

	fileContents := "this is a test"
	fileLocator := ArtifactDescriptor("cache", "foobar")

	t.Run("initialize", func(t *testing.T) {
		var err error
		client, err = NewS3(bucketName)
		require.NoError(t, err)
	})

	t.Run("set artifact", func(t *testing.T) {
		data := []byte(fileContents)
		dataReader := bytes.NewReader(data)
		readerCloser := io.NopCloser(dataReader)

		err := client.Set(fileLocator, "text/plain", len(data), readerCloser)
		require.NoError(t, err)
	})

	t.Run("read artifact", func(t *testing.T) {
		meta, reader, err := client.Get(fileLocator)
		require.NoError(t, err)
		assert.Equal(t, len(fileContents), meta.Size)
		assert.Equal(t, "text/plain", meta.Mime)
		assert.InDelta(t, time.Now().UTC().Unix(), meta.CreatedAt.Unix(), 10)
		assert.InDelta(t, time.Now().UTC().Unix(), meta.AccessedAt.Unix(), 10)

		data, err := io.ReadAll(reader)
		_ = reader.Close()
		require.NoError(t, err)

		assert.Equal(t, fileContents, string(data))
	})

	t.Run("read artifact (not found)", func(t *testing.T) {
		meta, reader, err := client.Get(ArtifactDescriptor("cache", "foobars"))
		assert.Nil(t, meta)
		assert.Nil(t, reader)
		assert.ErrorAs(t, err, &ErrNotExist{})
	})

	t.Run("touch", func(t *testing.T) {
		currentMeta, err := client.MetadataOf(fileLocator)
		require.NoError(t, err)
		time.Sleep(1 * time.Second)
		err = client.Touch(fileLocator)
		require.NoError(t, err)

		newMeta, err := client.MetadataOf(fileLocator)
		require.NoError(t, err)

		assert.Equal(t, currentMeta.CreatedAt, newMeta.CreatedAt)
		assert.NotEqual(t, currentMeta.AccessedAt, newMeta.AccessedAt)
	})

	t.Run("total size", func(t *testing.T) {
		// Create 2k Artifacts so we have something to iterate over
		thence := time.Now()
		for i := 0; i < 2000; i++ {
			data := []byte(fileContents)
			dataReader := bytes.NewReader(data)
			readerCloser := io.NopCloser(dataReader)
			err := client.Set(ArtifactDescriptor("cache", "cache-key-"+strconv.Itoa(i)), "text/plain", len(fileContents), readerCloser)
			require.NoError(t, err)
		}
		delta := time.Since(thence)
		fmt.Printf("Injected 2k items into S3 in %s; %.3f item(s)/sec\n", delta, 2000.0/float64(delta.Seconds()))
		// ---------
		size, err := client.TotalSize(KindArtifact)
		require.NoError(t, err)
		assert.Equal(t, int64(len(fileContents)*2001), size)
	})

	t.Run("delete group", func(t *testing.T) {
		data := []byte(fileContents)
		dataReader := bytes.NewReader(data)
		readerCloser := io.NopCloser(dataReader)

		err := client.Set(fileLocator, "text/plain", len(data), readerCloser)
		require.NoError(t, err)

		err = client.DeleteGroup(ArtifactParentDescriptor("cache"))
		assert.NoError(t, err)

		list, err := s3Client.ListObjects(context.Background(), &s3.ListObjectsInput{Bucket: &bucketName})
		assert.NoError(t, err)
		assert.Empty(t, list.Contents)
	})

	t.Run("delete", func(t *testing.T) {
		data := []byte(fileContents)
		dataReader := bytes.NewReader(data)
		readerCloser := io.NopCloser(dataReader)
		err := client.Set(fileLocator, "text/plain", len(fileContents), readerCloser)
		require.NoError(t, err)

		err = client.Delete(fileLocator)
		assert.NoError(t, err)

		list, err := s3Client.ListObjects(context.Background(), &s3.ListObjectsInput{Bucket: &bucketName})
		assert.NoError(t, err)
		assert.Empty(t, list.Contents)
	})

}
