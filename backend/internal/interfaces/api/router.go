package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"go.uber.org/zap"
)

// NewRouter builds the shared Gin engine used by all apps.
func NewRouter(logger *zap.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.LoggerWithWriter(gin.DefaultWriter))
	router.Use(middleware.Recovery())
	router.Use(middleware.CORS())
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
	logger.Info("router initialized")
	return router
}
