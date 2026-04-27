package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New создаёт структурный zap-логгер. В dev — консольный с цветами, в prod — JSON для агрегаторов.
func New(env string) (*zap.Logger, error) {
	if env == "prod" {
		return zap.NewProduction()
	}

	cfg := zap.NewDevelopmentConfig()
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	return cfg.Build()
}
