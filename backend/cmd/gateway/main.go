package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	api "github.com/lgt/asr/internal/interfaces/api"
	pkgconfig "github.com/lgt/asr/pkg/config"
	"go.uber.org/zap"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

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
	registerWSProxy(router, "/ws/events", cfg.Gateway.ASRAPI, logger)
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
	router.Any(prefix, func(c *gin.Context) {
		proxy.ServeHTTP(c.Writer, c.Request)
	})
	router.Any(prefix+"/*path", func(c *gin.Context) {
		proxy.ServeHTTP(c.Writer, c.Request)
	})
}

// registerWSProxy sets up a true WebSocket-level proxy that upgrades both
// the client connection and the upstream connection using gorilla/websocket,
// then forwards frames bidirectionally. This avoids the known limitations
// of httputil.ReverseProxy when proxying WebSocket through Gin.
func registerWSProxy(router *gin.Engine, prefix, target string, logger *zap.Logger) {
	handler := func(c *gin.Context) {
		upstreamURL := buildWSUpstreamURL(target, c.Request, prefix)

		upstreamConn, _, err := websocket.DefaultDialer.Dial(upstreamURL, nil)
		if err != nil {
			logger.Warn("ws proxy: failed to dial upstream", zap.String("url", upstreamURL), zap.Error(err))
			c.JSON(http.StatusBadGateway, gin.H{"error": "upstream websocket unavailable"})
			return
		}
		defer upstreamConn.Close()

		clientConn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			logger.Warn("ws proxy: failed to upgrade client", zap.Error(err))
			return
		}
		defer clientConn.Close()

		errc := make(chan error, 2)

		// upstream → client
		go func() {
			for {
				msgType, payload, readErr := upstreamConn.ReadMessage()
				if readErr != nil {
					errc <- readErr
					return
				}
				if writeErr := clientConn.WriteMessage(msgType, payload); writeErr != nil {
					errc <- writeErr
					return
				}
			}
		}()

		// client → upstream
		go func() {
			for {
				msgType, payload, readErr := clientConn.ReadMessage()
				if readErr != nil {
					errc <- readErr
					return
				}
				if writeErr := upstreamConn.WriteMessage(msgType, payload); writeErr != nil {
					errc <- writeErr
					return
				}
			}
		}()

		<-errc
	}

	router.GET(prefix, handler)
}

// buildWSUpstreamURL converts the HTTP target URL and incoming request into
// a ws:// or wss:// URL that includes the original path and query params.
func buildWSUpstreamURL(target string, req *http.Request, prefix string) string {
	parsed, _ := url.Parse(target)
	scheme := "ws"
	if parsed.Scheme == "https" {
		scheme = "wss"
	}

	path := prefix
	if strings.TrimSpace(req.URL.RawQuery) != "" {
		return fmt.Sprintf("%s://%s%s?%s", scheme, parsed.Host, path, req.URL.RawQuery)
	}
	return fmt.Sprintf("%s://%s%s", scheme, parsed.Host, path)
}
