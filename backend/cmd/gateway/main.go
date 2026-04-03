package main

import (
	"fmt"
	"log"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
	api "github.com/lgt/asr/internal/interfaces/api"
	pkgconfig "github.com/lgt/asr/pkg/config"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	cfg, err := pkgconfig.Load("configs/config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	router := api.NewRouter(logger)
	registerProxy(router, "/api/asr", cfg.Gateway.ASRAPI)
	registerProxy(router, "/api/meetings", cfg.Gateway.ASRAPI)
	registerProxy(router, "/uploads", cfg.Gateway.ASRAPI)
	registerProxy(router, "/ws/events", cfg.Gateway.ASRAPI)
	registerProxy(router, "/ws/transcribe", cfg.Gateway.ASRAPI)
	registerProxy(router, "/api/admin", cfg.Gateway.AdminAPI)
	registerProxy(router, "/api/nlp", cfg.Gateway.NLPAPI)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Info("gateway listening", zap.String("addr", addr))
	log.Fatal(router.Run(addr))
}

func registerProxy(router *gin.Engine, prefix, target string) {
	upstream, err := url.Parse(target)
	if err != nil {
		panic(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	router.Any(prefix+"/*path", func(c *gin.Context) {
		proxy.ServeHTTP(c.Writer, c.Request)
	})
	if prefix == "/ws/transcribe" || prefix == "/ws/events" || prefix == "/uploads" {
		router.Any(prefix, func(c *gin.Context) {
			proxy.ServeHTTP(c.Writer, c.Request)
		})
	}
}
