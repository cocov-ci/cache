package server

import (
	"fmt"
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

func (c *Config) MakeProvider() (*Provider, error) {
	prov, err := c.storageProvider()
	if err != nil {
		return nil, err
	}

	authenticator := func(r *http.Request) error {
		ok, repoName, err := c.RedisClient.RepoNameFromJID(r.Header.Get("Cocov-Job-ID"))
		if err != nil {
			return err
		}

		if !ok || repoName == "" {
			return fmt.Errorf("invalid or expired job id")
		}

		return nil
	}

	srv := &Provider{
		Logger:          c.Logger,
		StorageProvider: prov,
		Redis:           c.RedisClient,
		Authenticator:   authenticator,
		MaxPackageSize:  c.MaxPackageSize,
	}

	return srv, nil
}

func (c *Config) storageProvider() (storage.Provider, error) {
	var (
		provider        storage.Provider
		storageLogField zap.Field
		err             error
	)

	if c.StorageMode == "local" {
		provider, err = storage.NewLocalStorage(c.LocalStoragePath)
		storageLogField = zap.String("local_storage_path", c.LocalStoragePath)

	} else {
		provider, err = storage.NewS3(c.S3BucketName)
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
}

func (p *Provider) makeArtifactHandler() *Handler {
	return &Handler{
		Authenticator: p.Authenticator,
		LocatorGenerator: func(h *Handler, r *http.Request, id string) (storage.ObjectDescriptor, error) {
			ok, repoName, err := h.Redis.RepoNameFromJID(r.Header.Get("Cocov-Job-ID"))
			if err != nil {
				return nil, err
			}

			if !ok || repoName == "" {
				return nil, fmt.Errorf("invalid or expired job id")
			}

			return storage.ArtifactDescriptor(repoName, id), nil
		},
		MaxPackageSize: p.MaxPackageSize,
		Provider:       p.StorageProvider,
		Redis:          p.Redis,
	}
}

func (p *Provider) makeToolHandler() *Handler {
	return &Handler{
		Authenticator: p.Authenticator,
		LocatorGenerator: func(h *Handler, r *http.Request, id string) (storage.ObjectDescriptor, error) {
			return storage.ToolDescriptor(id), nil
		},
		MaxPackageSize: p.MaxPackageSize,
		Provider:       p.StorageProvider,
		Redis:          p.Redis,
	}
}

func (p *Provider) MakeHandler(kind storage.Kind) *Handler {
	if kind == storage.KindArtifact {
		return p.makeArtifactHandler()
	}

	return p.makeToolHandler()
}

func (p *Provider) MakeMux() *chi.Mux {
	artifactHandler := p.MakeHandler(storage.KindArtifact)
	toolsHandler := p.MakeHandler(storage.KindTool)

	r := chi.NewRouter()
	r.Use(logging.RequestLogger(p.Logger))

	r.Get("/artifact/{id}", artifactHandler.HandleGet)
	r.Head("/artifact/{id}", artifactHandler.HandleHead)
	r.Post("/artifact/{id}", artifactHandler.HandleSet)

	r.Get("/tool/{id}", toolsHandler.HandleGet)
	r.Head("/tool/{id}", toolsHandler.HandleHead)
	r.Post("/tool/{id}", toolsHandler.HandleSet)

	return r
}
