package server

import (
	"fmt"
	"github.com/cocov-ci/cache/api"
	"github.com/cocov-ci/cache/locator"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/cocov-ci/cache/logging"
	"github.com/cocov-ci/cache/redis"
	"github.com/cocov-ci/cache/storage"
)

type Config struct {
	Logger           *zap.Logger
	StorageMode      string
	LocalStoragePath string
	S3BucketName     string
	BindAddress      string
	MaxPackageSize   int64
	RedisClient      redis.Client
}

func (c *Config) authenticator(r *http.Request) error {
	id, err := c.RedisClient.RepoDataFromJID(r.Header.Get("Cocov-Job-ID"))
	if err != nil {
		return err
	}

	if id == nil {
		return fmt.Errorf("invalid or expired job id")
	}

	return nil
}

func (c *Config) MakeProvider(apiClient api.Client) (*Provider, error) {
	prov, err := c.storageProvider(apiClient)
	if err != nil {
		return nil, err
	}

	srv := &Provider{
		Logger:          c.Logger,
		StorageProvider: prov,
		Redis:           c.RedisClient,
		Authenticator:   c.authenticator,
		MaxPackageSize:  c.MaxPackageSize,
	}

	srv.artifactHandler = MakeArtifactHandler(c, prov)
	srv.toolsHandler = MakeToolsHandler(c, prov)

	return srv, nil
}

func (c *Config) storageProvider(apiClient api.Client) (storage.Provider, error) {
	var (
		provider        storage.Provider
		storageLogField zap.Field
		err             error
	)

	if c.StorageMode == "local" {
		provider, err = storage.NewLocalStorage(c.LocalStoragePath, apiClient)
		storageLogField = zap.String("local_storage_path", c.LocalStoragePath)

	} else {
		provider, err = storage.NewS3(c.S3BucketName, apiClient)
		storageLogField = zap.String("bucket_name", c.S3BucketName)
	}

	if err != nil {
		c.Logger.Error("Storage provider initialization failed", zap.Error(err))
		return nil, err
	}

	c.Logger.Info("Storage provider initialization succeeded",
		zap.String("provider_kind", c.StorageMode),
		storageLogField,
	)

	return provider, nil
}

type Provider struct {
	Logger          *zap.Logger
	StorageProvider storage.Provider
	Redis           redis.Client
	Authenticator   AuthenticatorFunc
	MaxPackageSize  int64
	artifactHandler Handler[*locator.ArtifactLocator]
	toolsHandler    Handler[*locator.ToolLocator]
}

func MakeArtifactHandler(c *Config, p storage.Provider) Handler[*locator.ArtifactLocator] {
	return Handler[*locator.ArtifactLocator]{
		Authenticator:  c.authenticator,
		MaxPackageSize: c.MaxPackageSize,
		Redis:          c.RedisClient,
		HandleGet: func(loc *locator.ArtifactLocator, r *http.Request) (size int64, mime string, stream io.ReadCloser, err error) {
			meta, stream, err := p.GetArtifact(loc)
			if err != nil {
				return 0, "", nil, err
			}

			return meta.Size, meta.Mime, stream, err
		},
		HandleHead: func(loc *locator.ArtifactLocator, r *http.Request) (size int64, mime string, err error) {
			meta, err := p.GetArtifactMeta(loc)
			if err != nil {
				return 0, "", err
			}
			return meta.Size, meta.Mime, err
		},
		HandleSet: func(loc *locator.ArtifactLocator, mime string, size int64, stream io.ReadCloser) error {
			return p.SetArtifact(loc, mime, size, stream)
		},
	}
}

func MakeToolsHandler(c *Config, p storage.Provider) Handler[*locator.ToolLocator] {
	return Handler[*locator.ToolLocator]{
		Authenticator:  c.authenticator,
		MaxPackageSize: c.MaxPackageSize,
		Redis:          c.RedisClient,
		HandleGet: func(loc *locator.ToolLocator, r *http.Request) (size int64, mime string, stream io.ReadCloser, err error) {
			meta, stream, err := p.GetTool(loc)
			if err != nil {
				return 0, "", nil, err
			}

			return meta.Size, meta.Mime, stream, err
		},
		HandleHead: func(loc *locator.ToolLocator, r *http.Request) (size int64, mime string, err error) {
			meta, err := p.GetToolMeta(loc)
			if err != nil {
				return 0, "", err
			}
			return meta.Size, meta.Mime, err
		},
		HandleSet: func(loc *locator.ToolLocator, mime string, size int64, stream io.ReadCloser) error {
			return p.SetTool(loc, mime, size, stream)
		},
	}
}

func (p *Provider) MakeMux() *chi.Mux {
	r := chi.NewRouter()
	r.Use(logging.RequestLogger(p.Logger))

	r.Get("/artifact/{id}", p.artifactHandler.Get)
	r.Head("/artifact/{id}", p.artifactHandler.Head)
	r.Post("/artifact/{id}", p.artifactHandler.Set)

	r.Get("/tool/{id}", p.toolsHandler.Get)
	r.Head("/tool/{id}", p.toolsHandler.Head)
	r.Post("/tool/{id}", p.toolsHandler.Set)

	return r
}
