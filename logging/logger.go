package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func InitializeLogger(isDevelopment bool) (*zap.Logger, error) {
	var logger *zap.Logger
	var err error
	if isDevelopment {
		config := zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.Development = true
		logger, err = config.Build()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		return nil, err
	}

	zap.ReplaceGlobals(logger)

	return logger, nil
}
