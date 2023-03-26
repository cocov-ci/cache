//go:build unit

package server

import (
	"fmt"
	"github.com/cocov-ci/cache/api"
	"github.com/cocov-ci/cache/locator"
	"github.com/cocov-ci/cache/redis"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/heyvito/httptest-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/cocov-ci/cache/logging"
	"github.com/cocov-ci/cache/mocks"
	"github.com/cocov-ci/cache/storage"
)

type MockList struct {
	StorageProvider *mocks.MockProvider
	Redis           *mocks.MockRedisClient
}

func MakeHandler(t *testing.T, maxPackageSize int64, auth AuthenticatorFunc) (*MockList, Handler[*locator.ArtifactLocator]) {
	ctrl := gomock.NewController(t)
	redis := mocks.NewMockRedisClient(ctrl)
	store := mocks.NewMockProvider(ctrl)

	list := &MockList{
		StorageProvider: store,
		Redis:           redis,
	}

	hand := MakeArtifactHandler(&Config{
		Logger:         zap.NewNop(),
		MaxPackageSize: maxPackageSize,
		RedisClient:    redis,
	}, store)
	hand.Authenticator = auth

	return list, hand
}

var nopLoggerMiddleware = logging.RequestLogger(zap.NewNop())

func TestHandler_HandleGet(t *testing.T) {
	exec := func(req *http.Request, hand Handler[*locator.ArtifactLocator]) *http.Response {
		return httptest.ExecuteMiddlewareWithRequest(req, http.HandlerFunc(hand.Get), nopLoggerMiddleware)
	}

	t.Run("without a Cocov-Job-ID", func(t *testing.T) {
		_, hand := MakeHandler(t, 0, nil)
		req := httptest.PrepareRequest(httptest.EmptyRequest())
		resp := exec(req, hand)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.Equal(t, "Missing job identifier", string(ResponseData(t, resp)))
	})

	t.Run("Authentication rejection", func(t *testing.T) {
		_, hand := MakeHandler(t, 0, func(_ *http.Request) error {
			return fmt.Errorf("nope")
		})
		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.Equal(t, "Forbidden", string(ResponseData(t, resp)))
	})

	t.Run("Missing ID", func(o *testing.T) {
		_, hand := MakeHandler(t, 0, nil)
		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Equal(t, "Missing ID", string(ResponseData(t, resp)))
	})

	t.Run("Locator Generator failure", func(t *testing.T) {
		mock, hand := MakeHandler(t, 0, nil)
		mock.Redis.EXPECT().RepoDataFromJID("some-id").Return(nil, fmt.Errorf("boom"))

		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithURLParam("id", "hello"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Equal(t, "Internal server error", string(ResponseData(t, resp)))
	})

	t.Run("Storage item not found", func(t *testing.T) {
		mock, hand := MakeHandler(t, 0, nil)
		mock.Redis.EXPECT().RepoDataFromJID("some-id").Return(&redis.RepoIdentifier{ID: 1, Name: "bla"}, nil)

		mock.Redis.EXPECT().Locking("artifact:1:hello", gomock.Any(), gomock.Any()).DoAndReturn(func(_ string, _ time.Duration, fn func() error) error {
			return fn()
		})

		mock.StorageProvider.EXPECT().GetArtifact(gomock.Any()).Return(nil, nil, storage.ErrNotExist{})

		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithURLParam("id", "hello"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.Equal(t, "Not found", string(ResponseData(t, resp)))
	})

	t.Run("Locking or Storage generic failure", func(t *testing.T) {
		mock, hand := MakeHandler(t, 0, nil)
		mock.Redis.EXPECT().RepoDataFromJID("some-id").Return(&redis.RepoIdentifier{ID: 1, Name: "bla"}, nil)
		mock.Redis.EXPECT().Locking("artifact:1:hello", gomock.Any(), gomock.Any()).Return(fmt.Errorf("boom"))

		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithURLParam("id", "hello"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Equal(t, "Internal server error", string(ResponseData(t, resp)))
	})

	t.Run("Success", func(t *testing.T) {
		mock, hand := MakeHandler(t, 0, nil)

		mock.Redis.EXPECT().RepoDataFromJID("some-id").Return(&redis.RepoIdentifier{ID: 1, Name: "bla"}, nil)
		mock.Redis.EXPECT().Locking("artifact:1:hello", gomock.Any(), gomock.Any()).DoAndReturn(func(_ string, _ time.Duration, fn func() error) error {
			return fn()
		})

		mock.StorageProvider.EXPECT().GetArtifact(gomock.Any()).Return(&api.GetArtifactMetaOutput{
			CreatedAt:  time.Time{},
			LastUsedAt: time.Time{},
			Size:       14,
			Mime:       "text/plain",
			Name:       "hello",
			ID:         1,
			Engine:     "local",
		}, io.NopCloser(strings.NewReader("this is a test")), nil)

		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithURLParam("id", "hello"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, int64(14), resp.ContentLength)
		assert.Equal(t, "text/plain", resp.Header.Get("Content-Type"))
		assert.Equal(t, "this is a test", string(ResponseData(t, resp)))
	})
}

func TestHandler_HandleHead(t *testing.T) {
	exec := func(req *http.Request, hand Handler[*locator.ArtifactLocator]) *http.Response {
		return httptest.ExecuteMiddlewareWithRequest(req, http.HandlerFunc(hand.Head), nopLoggerMiddleware)
	}
	t.Run("without a Cocov-Job-ID", func(t *testing.T) {
		_, hand := MakeHandler(t, 0, nil)
		req := httptest.PrepareRequest(httptest.EmptyRequest())
		resp := exec(req, hand)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("Authentication rejection", func(t *testing.T) {
		_, hand := MakeHandler(t, 0, func(_ *http.Request) error {
			return fmt.Errorf("nope")
		})
		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("Missing ID", func(o *testing.T) {
		_, hand := MakeHandler(t, 0, nil)
		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Locator Generator failure", func(t *testing.T) {
		mock, hand := MakeHandler(t, 0, nil)
		mock.Redis.EXPECT().RepoDataFromJID("some-id").Return(nil, fmt.Errorf("boom"))

		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithURLParam("id", "hello"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("Storage item not found", func(t *testing.T) {
		mock, hand := MakeHandler(t, 0, nil)
		mock.Redis.EXPECT().RepoDataFromJID("some-id").Return(&redis.RepoIdentifier{ID: 1, Name: "bla"}, nil)
		mock.Redis.EXPECT().Locking("artifact:1:hello", gomock.Any(), gomock.Any()).DoAndReturn(func(_ string, _ time.Duration, fn func() error) error {
			return fn()
		})

		mock.StorageProvider.EXPECT().GetArtifactMeta(gomock.Any()).Return(nil, storage.ErrNotExist{})

		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithURLParam("id", "hello"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("Locking or Storage generic failure", func(t *testing.T) {
		mock, hand := MakeHandler(t, 0, nil)
		mock.Redis.EXPECT().RepoDataFromJID("some-id").Return(&redis.RepoIdentifier{ID: 1, Name: "bla"}, nil)
		mock.Redis.EXPECT().Locking("artifact:1:hello", gomock.Any(), gomock.Any()).Return(fmt.Errorf("boom"))

		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithURLParam("id", "hello"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("Success", func(t *testing.T) {
		mock, hand := MakeHandler(t, 0, nil)
		mock.Redis.EXPECT().RepoDataFromJID("some-id").Return(&redis.RepoIdentifier{ID: 1, Name: "bla"}, nil)
		mock.Redis.EXPECT().Locking("artifact:1:hello", gomock.Any(), gomock.Any()).DoAndReturn(func(_ string, _ time.Duration, fn func() error) error {
			return fn()
		})

		mock.StorageProvider.EXPECT().GetArtifactMeta(gomock.Any()).Return(&api.GetArtifactMetaOutput{
			CreatedAt:  time.Time{},
			LastUsedAt: time.Time{},
			Size:       14,
			Mime:       "text/plain",
			ID:         1,
			Name:       "test",
		}, nil)

		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithURLParam("id", "hello"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, int64(14), resp.ContentLength)
		assert.Equal(t, "text/plain", resp.Header.Get("Content-Type"))
	})
}

func TestHandler_HandleSet(t *testing.T) {
	exec := func(req *http.Request, hand Handler[*locator.ArtifactLocator]) *http.Response {
		return httptest.ExecuteMiddlewareWithRequest(req, http.HandlerFunc(hand.Set), nopLoggerMiddleware)
	}
	t.Run("without a Cocov-Job-ID", func(t *testing.T) {
		_, hand := MakeHandler(t, 0, nil)
		req := httptest.PrepareRequest(httptest.EmptyRequest())
		resp := exec(req, hand)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.Equal(t, "Missing job identifier", string(ResponseData(t, resp)))
	})

	t.Run("Authentication rejection", func(t *testing.T) {
		_, hand := MakeHandler(t, 0, func(_ *http.Request) error {
			return fmt.Errorf("nope")
		})
		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.Equal(t, "Forbidden", string(ResponseData(t, resp)))
	})

	t.Run("Missing Content-Type", func(t *testing.T) {
		_, hand := MakeHandler(t, 0, nil)
		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Equal(t, "Missing Content-Type", string(ResponseData(t, resp)))
	})

	t.Run("Missing Content-Length", func(t *testing.T) {
		_, hand := MakeHandler(t, 0, nil)
		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithHeader("Content-Type", "text/plain"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Equal(t, "Missing Content-Length", string(ResponseData(t, resp)))
	})

	t.Run("Missing filename", func(t *testing.T) {
		_, hand := MakeHandler(t, 0, nil)
		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithHeader("Content-Type", "text/plain"),
			httptest.WithContentLength(6),
			httptest.WithBodyString("Hello!"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Equal(t, "Missing Filename", string(ResponseData(t, resp)))
	})

	t.Run("Request Too Large", func(t *testing.T) {
		_, hand := MakeHandler(t, int64(len("Hello")-1), nil)
		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithHeader("Content-Type", "text/plain"),
			httptest.WithHeader("Filename", "foo.bar"),
			httptest.WithContentLength(6),
			httptest.WithBodyString("Hello!"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
		assert.Equal(t, "Request entity too large", string(ResponseData(t, resp)))
	})

	t.Run("Missing ID", func(t *testing.T) {
		_, hand := MakeHandler(t, 0, nil)
		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithHeader("Content-Type", "text/plain"),
			httptest.WithHeader("Filename", "foo.bar"),
			httptest.WithContentLength(6),
			httptest.WithBodyString("Hello!"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Equal(t, "Missing ID", string(ResponseData(t, resp)))
	})

	t.Run("Locator failure", func(t *testing.T) {
		mock, hand := MakeHandler(t, 0, nil)
		mock.Redis.EXPECT().RepoDataFromJID("some-id").Return(nil, fmt.Errorf("boom"))

		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithHeader("Content-Type", "text/plain"),
			httptest.WithHeader("Filename", "foo.bar"),
			httptest.WithContentLength(6),
			httptest.WithBodyString("Hello!"),
			httptest.WithURLParam("id", "foobar"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Equal(t, "Internal server error", string(ResponseData(t, resp)))
	})

	t.Run("Write failure", func(t *testing.T) {
		mock, hand := MakeHandler(t, 0, nil)
		mock.Redis.EXPECT().RepoDataFromJID("some-id").Return(&redis.RepoIdentifier{ID: 1, Name: "bla"}, nil)
		mock.Redis.EXPECT().Locking("artifact:1:foobar", gomock.Any(), gomock.Any()).DoAndReturn(func(_ string, _ time.Duration, fn func() error) error {
			return fn()
		})

		mock.StorageProvider.EXPECT().SetArtifact(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("boom"))

		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithHeader("Content-Type", "text/plain"),
			httptest.WithHeader("Filename", "foo.bar"),
			httptest.WithContentLength(6),
			httptest.WithBodyString("Hello!"),
			httptest.WithURLParam("id", "foobar"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Equal(t, "Internal server error", string(ResponseData(t, resp)))
	})

	t.Run("Successful write", func(t *testing.T) {
		mock, hand := MakeHandler(t, 0, nil)
		mock.Redis.EXPECT().RepoDataFromJID("some-id").Return(&redis.RepoIdentifier{ID: 1, Name: "bla"}, nil)
		mock.Redis.EXPECT().Locking("artifact:1:foobar", gomock.Any(), gomock.Any()).DoAndReturn(func(_ string, _ time.Duration, fn func() error) error {
			return fn()
		})

		mock.StorageProvider.EXPECT().SetArtifact(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Do(func(_ *locator.ArtifactLocator, cType string, len int64, reader io.ReadCloser) {
			assert.Equal(t, "text/plain", cType)
			assert.Equal(t, int64(6), len)
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			assert.Equal(t, "Hello!", string(data))
		})

		req := httptest.PrepareRequest(httptest.EmptyRequest(),
			httptest.WithHeader("Cocov-Job-ID", "some-id"),
			httptest.WithHeader("Content-Type", "text/plain"),
			httptest.WithContentLength(6),
			httptest.WithBodyString("Hello!"),
			httptest.WithHeader("Filename", "foo.bar"),
			httptest.WithURLParam("id", "foobar"))
		resp := exec(req, hand)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})
}
