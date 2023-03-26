//go:build unit

package storage

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cocov-ci/cache/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

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

func makeS3Client(t *testing.T) (string, *s3.Client, Provider) {
	var bucketName = fmt.Sprintf("cocov-cache-test-%d", time.Now().UTC().UnixMilli())
	var env = map[string]string{
		"AWS_ACCESS_KEY_ID":     "minioadmin",
		"AWS_SECRET_ACCESS_KEY": "minioadmin",
		"AWS_REGION":            "us-east-1",
	}

	if os.Getenv("COCOV_CACHE_S3_ENDPOINT") == "" {
		require.NoError(t, os.Setenv("COCOV_CACHE_S3_ENDPOINT", "http://localhost:9000"))
	}

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

	apiURL := os.Getenv("API_URL")
	if len(apiURL) == 0 {
		apiURL = "http://localhost:3000"
	}
	apiToken := os.Getenv("API_TOKEN")
	apiClient := api.New(apiURL, apiToken)

	s, err := NewS3(bucketName, apiClient)
	require.NoError(t, err)
	return bucketName, s3Client, s
}

func countBucketItems(t *testing.T, client *s3.Client, bucket string) int {
	list, err := client.ListObjects(context.Background(), &s3.ListObjectsInput{
		Bucket:  &bucket,
		MaxKeys: 1000,
	})

	require.NoError(t, err)
	return len(list.Contents)
}

func TestS3(t *testing.T) {
	t.Run("set artifact", func(t *testing.T) {
		bucketName, client, provider := makeS3Client(t)
		testSetArtifact(t, provider, func(t *testing.T) int {
			return countBucketItems(t, client, bucketName)
		})
	})

	t.Run("read artifact", func(t *testing.T) {
		_, _, provider := makeS3Client(t)
		testReadArtifact(t, provider)
	})

	t.Run("read artifact (not found)", func(t *testing.T) {
		_, _, provider := makeS3Client(t)
		testArtifactNotFound(t, provider)
	})

	t.Run("touch artifact", func(t *testing.T) {
		_, _, provider := makeS3Client(t)
		testArtifactTouch(t, provider)
	})

	t.Run("delete artifact", func(t *testing.T) {
		bucketName, client, provider := makeS3Client(t)
		testArtifactDelete(t, provider, func(t *testing.T) int {
			return countBucketItems(t, client, bucketName)
		})
	})

	t.Run("purge repository", func(t *testing.T) {
		bucketName, client, provider := makeS3Client(t)
		testPurgeRepository(t, provider, func(t *testing.T) int {
			return countBucketItems(t, client, bucketName)
		})
	})

	t.Run("set tool", func(t *testing.T) {
		bucketName, client, provider := makeS3Client(t)
		testSetTool(t, provider, func(t *testing.T) int {
			return countBucketItems(t, client, bucketName)
		})
	})

	t.Run("read tool", func(t *testing.T) {
		_, _, provider := makeS3Client(t)
		testReadTool(t, provider)
	})

	t.Run("read tool (not found)", func(t *testing.T) {
		_, _, provider := makeS3Client(t)
		testToolNotFound(t, provider)
	})

	t.Run("touch tool", func(t *testing.T) {
		_, _, provider := makeS3Client(t)
		testToolTouch(t, provider)
	})

	t.Run("delete tool", func(t *testing.T) {
		bucketName, client, provider := makeS3Client(t)
		testToolDelete(t, provider, func(t *testing.T) int {
			return countBucketItems(t, client, bucketName)
		})
	})
}
