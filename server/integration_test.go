//go:build integration

package server

import (
	"bytes"
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cocov-ci/cache/api"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/heyvito/httptest-go"
	redis2 "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/cocov-ci/cache/redis"
)

func ExecTest(t *testing.T, config Config) {
	redisURL := os.Getenv("REDIS_URL")
	if len(redisURL) == 0 {
		redisURL = "redis://localhost:6379/0"
	}
	apiURL := os.Getenv("API_URL")
	if len(apiURL) == 0 {
		apiURL = "http://localhost:3000"
	}
	apiToken := os.Getenv("API_TOKEN")
	apiClient := api.New(apiURL, apiToken)

	err := apiClient.Ping()
	require.NoError(t, err)

	redisOpts, err := redis2.ParseURL(redisURL)
	require.NoError(t, err)
	realRedis := redis2.NewClient(redisOpts)
	red, err := redis.New(redisURL)
	require.NoError(t, err)

	log, err := zap.NewDevelopment()
	require.NoError(t, err)
	config.Logger = log
	config.RedisClient = red

	makeProv := func(t *testing.T) *chi.Mux {
		prov, err := config.MakeProvider(apiClient)
		require.NoError(t, err)
		return prov.MakeMux()
	}

	makeJobID := func(t *testing.T) httptest.RequestMutator {
		jid := uuid.NewString()
		err := realRedis.Set(context.Background(), "cocov:cached:client:"+jid, `{"id":1,"name":"cache"}`, 0).Err()
		require.NoError(t, err)
		return httptest.WithHeader("Cocov-Job-ID", jid)
	}

	contents := []byte(uuid.NewString())

	makePostRequest := func(t *testing.T, path string) *http.Request {
		req, err := http.NewRequest("POST", path, bytes.NewReader(contents))
		require.NoError(t, err)
		return req
	}

	makeGetRequest := func(t *testing.T, path string) *http.Request {
		req, err := http.NewRequest("GET", path, nil)
		require.NoError(t, err)
		return req
	}

	makeHeadRequest := func(t *testing.T, path string) *http.Request {
		req, err := http.NewRequest("HEAD", path, nil)
		require.NoError(t, err)
		return req
	}

	t.Run("Artifacts", func(t *testing.T) {
		t.Run("Set", func(t *testing.T) {
			mux := makeProv(t)
			req := httptest.PrepareRequest(makePostRequest(t, "/artifact/foobar"),
				makeJobID(t),
				httptest.WithHeader("Content-Type", "text/plain"),
				httptest.WithHeader("Filename", "foo.bar"))
			res := httptest.ExecuteRequest(req, mux.ServeHTTP)
			assert.Equal(t, http.StatusNoContent, res.StatusCode)
		})

		t.Run("Get", func(t *testing.T) {
			mux := makeProv(t)
			req := httptest.PrepareRequest(makeGetRequest(t, "/artifact/foobar"),
				makeJobID(t))

			res := httptest.ExecuteRequest(req, mux.ServeHTTP)
			assert.Equal(t, http.StatusOK, res.StatusCode)
			assert.Equal(t, contents, ResponseData(t, res))
		})

		t.Run("Head", func(t *testing.T) {
			mux := makeProv(t)
			req := httptest.PrepareRequest(makeHeadRequest(t, "/artifact/foobar"),
				makeJobID(t))

			res := httptest.ExecuteRequest(req, mux.ServeHTTP)
			assert.Equal(t, http.StatusOK, res.StatusCode)
			assert.Equal(t, "text/plain", res.Header.Get("Content-Type"))
			assert.Equal(t, int64(len(contents)), res.ContentLength)
		})
	})

	t.Run("Tools", func(t *testing.T) {
		t.Run("Set", func(t *testing.T) {
			mux := makeProv(t)
			req := httptest.PrepareRequest(makePostRequest(t, "/tool/foobar"),
				makeJobID(t),
				httptest.WithHeader("Content-Type", "text/plain"),
				httptest.WithHeader("Filename", "foo.bar"),
			)
			res := httptest.ExecuteRequest(req, mux.ServeHTTP)
			assert.Equal(t, http.StatusNoContent, res.StatusCode)
		})

		t.Run("Head", func(t *testing.T) {
			mux := makeProv(t)
			req := httptest.PrepareRequest(makeHeadRequest(t, "/tool/foobar"),
				makeJobID(t))

			res := httptest.ExecuteRequest(req, mux.ServeHTTP)
			assert.Equal(t, http.StatusOK, res.StatusCode)
			assert.Equal(t, "text/plain", res.Header.Get("Content-Type"))
			assert.Equal(t, int64(len(contents)), res.ContentLength)
		})

		t.Run("Get", func(t *testing.T) {
			mux := makeProv(t)
			req := httptest.PrepareRequest(makeGetRequest(t, "/tool/foobar"),
				makeJobID(t))

			res := httptest.ExecuteRequest(req, mux.ServeHTTP)
			assert.Equal(t, http.StatusOK, res.StatusCode)
			assert.Equal(t, contents, ResponseData(t, res))
		})
	})
}

func TestLocalIntegration(t *testing.T) {
	dir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	conf := Config{
		StorageMode:      "local",
		LocalStoragePath: dir,
	}
	ExecTest(t, conf)
}

func TestS3Integration(t *testing.T) {
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

	conf := Config{
		StorageMode:  "s3",
		S3BucketName: bucketName,
	}
	ExecTest(t, conf)
}
