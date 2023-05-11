package commands

import (
	"context"
	"fmt"
	"github.com/cocov-ci/cache/api"
	"github.com/cocov-ci/cache/housekeeping"
	"github.com/cocov-ci/cache/probes"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"github.com/cocov-ci/cache/logging"
	"github.com/cocov-ci/cache/redis"
	"github.com/cocov-ci/cache/server"
)

const attempts = 10

func Backoff(log *zap.Logger, operation string, fn func() error) (err error) {
	for i := 0; i < attempts; i++ {
		time.Sleep(time.Duration(i*4) * time.Second)
		if err = fn(); err != nil {
			log.Error("Backoff "+operation,
				zap.Int("delay_secs", (i+1)*4),
				zap.String("attempt", fmt.Sprintf("%d/%d", i+1, attempts)),
				zap.Error(err))
			continue
		}
		return nil
	}
	return err
}

func Serve(ctx *cli.Context) error {
	isDevelopment := os.Getenv("COCOV_WORKER_DEV") == "true"
	logger, err := logging.InitializeLogger(isDevelopment)
	if err != nil {
		return err
	}
	defer func() { _ = logger.Sync() }()

	logger.Info("Initializing probes server")
	probeSrv := probes.NewStatus(ctx.String("probes-server-bind-address"))
	go func() {
		if err := probeSrv.Run(); err != nil && err != http.ErrServerClosed {
			logger.Error("Probe server failed", zap.Error(err))
		}
	}()
	logger.Info("Initialized probes server")

	var redisClient redis.Client
	err = Backoff(logger, "initializing Redis client", func() error {
		redisClient, err = redis.New(ctx.String("redis-url"))
		return err
	})

	if err != nil {
		logger.Error("Gave trying to connect to Redis.")
		return err
	}

	logger.Info("Initialized Redis client")

	conf := &server.Config{
		Logger:           logger,
		RedisClient:      redisClient,
		StorageMode:      ctx.String("storage-mode"),
		LocalStoragePath: ctx.String("local-storage-path"),
		S3BucketName:     ctx.String("s3-bucket-name"),
		BindAddress:      ctx.String("bind-address"),
		MaxPackageSize:   ctx.Int64("max-package-size-bytes"),
	}

	apiClient := api.New(ctx.String("api-url"), ctx.String("api-token"))
	err = Backoff(logger, "initializing API client", func() error {
		return apiClient.Ping()
	})
	if err != nil {
		logger.Error("Gave up initializing API client")
		return err
	}

	logger.Info("Initialized API client")

	jan := housekeeping.New(redisClient, apiClient)
	go jan.Start()
	logger.Info("Initialized housekeeping services")

	p, err := conf.MakeProvider(apiClient)
	if err != nil {
		logger.Error("Failed creating Mux from configuration", zap.Error(err))
		return err
	}
	logger.Info("Pre-initialized server")

	httpServer := http.Server{
		Addr:    conf.BindAddress,
		Handler: p.MakeMux(),
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	shutdown := make(chan bool)
	go func() {
		<-signalChan
		logger.Info("Received interrupt signal. Gracefully stopping...")
		probeSrv.SetReady(false)
		jan.Stop()
		logger.Info("Stopping HTTP server...")
		err := httpServer.Shutdown(context.Background())
		if err != nil {
			logger.Error("Failed requesting HTTP server shutdown", zap.Error(err))
		}
		logger.Info("HTTP server stopped")
		close(shutdown)
	}()

	logger.Info("Starting HTTP server", zap.String("bind_address", conf.BindAddress))
	probeSrv.SetReady(true)
	if err = httpServer.ListenAndServe(); err != nil {
		<-shutdown
		if err != http.ErrServerClosed {
			return err
		}
	}
	<-shutdown

	logger.Info("Bye!")
	return nil
}
