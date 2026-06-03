package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// defaultAccessLogSkipPaths are health/heartbeat endpoints that should never be
// written to the access log. They are polled continuously and otherwise flood
// the logs with noise.
var defaultAccessLogSkipPaths = map[string]struct{}{
	"/healthz":    {},
	"/readyz":     {},
	"/api/health": {},
	"/api/ping":   {},
	"/ws/events":  {},
}

// AccessLog returns a gin middleware that logs HTTP requests through zap with
// levels layered by response status:
//   - 5xx -> Error
//   - 4xx -> Warn
//   - else -> Info
//
// Heartbeat/health paths are skipped so they do not pollute the logs.
func AccessLog(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if _, skip := defaultAccessLogSkipPaths[path]; skip {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", time.Since(start)),
			zap.String("ip", c.ClientIP()),
		}
		if msg := c.Errors.ByType(gin.ErrorTypePrivate).String(); msg != "" {
			fields = append(fields, zap.String("error", msg))
		}

		switch {
		case status >= 500:
			logger.Error("http request", fields...)
		case status >= 400:
			logger.Warn("http request", fields...)
		default:
			logger.Info("http request", fields...)
		}
	}
}
