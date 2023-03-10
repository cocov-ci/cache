package commands

import (
	"context"
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

func Serve(ctx *cli.Context) error {
	isDevelopment := os.Getenv("COCOV_WORKER_DEV") == "true"
	logger, err := logging.InitializeLogger(isDevelopment)
	if err != nil {
		return err
	}
	defer func() { _ = logger.Sync() }()

	var redisClient redis.Client
	for i := 0; i < 5; i++ {
		redisClient, err = redis.New(ctx.String("redis-url"))
		if err != nil {
			logger.Error("Failed initializing Redis client", zap.Error(err))
			delay := time.Duration(i*2) * time.Second
			logger.Info("Retrying", zap.Duration("delay", delay))
			time.Sleep(delay)
		} else {
			break
		}
	}

	if err != nil {
		logger.Error("Exhausted attempts to connect to Redis.")
		return err
	}

	conf := &server.Config{
		Logger:           logger,
		RedisClient:      redisClient,
		StorageMode:      ctx.String("storage-mode"),
		LocalStoragePath: ctx.String("local-storage-path"),
		S3BucketName:     ctx.String("s3-bucket-name"),
		BindAddress:      ctx.String("bind-address"),
		MaxPackageSize:   ctx.Int64("max-package-size-bytes"),
	}

	p, err := conf.MakeProvider()
	if err != nil {
		logger.Error("Failed creating Mux from configuration", zap.Error(err))
		return err
	}

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
		err := httpServer.Shutdown(context.Background())
		if err != nil {
			logger.Error("Failed requesting HTTP server shutdown", zap.Error(err))
		}
		close(shutdown)
	}()

	logger.Info("Starting HTTP server", zap.String("bind_address", conf.BindAddress))

	if err = httpServer.ListenAndServe(); err != nil {
		<-shutdown
		if err != http.ErrServerClosed {
			return err
		}
	}
	<-shutdown

	return nil
}
