// Package logging provides a shared, level-configurable zap logger used by all
// backend apps. It replaces the previous per-app zap.NewDevelopment() loggers,
// which emitted verbose debug output and stack traces by default.
package logging

import (
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New builds a production-style console logger whose minimum level is derived
// from the provided level string (debug, info, warn, error). Unknown values
// fall back to info.
func New(level string) *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(parseLevel(level))
	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	// Avoid noisy stack traces on every warning; only attach them to errors.
	cfg.DisableStacktrace = true

	logger, err := cfg.Build()
	if err != nil {
		// Fall back to a no-frills logger so startup never blocks on logging.
		return zap.NewNop()
	}
	return logger
}

func parseLevel(level string) zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return zapcore.DebugLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "info", "":
		return zapcore.InfoLevel
	default:
		return zapcore.InfoLevel
	}
}
