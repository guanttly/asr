package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	router.Use(legacyAccessLogger(cfg.Legacy.AccessLogPath))
	registerProxy(router, "/api/asr", cfg.Gateway.ASRAPI)
	registerProxy(router, "/api/meetings", cfg.Gateway.ASRAPI)
	registerProxy(router, "/uploads", cfg.Gateway.ASRAPI)
	registerWSProxy(router, "/ws/events", cfg.Gateway.ASRAPI, logger)
	registerProxy(router, "/api/admin", cfg.Gateway.AdminAPI)
	registerProxy(router, "/api/nlp", cfg.Gateway.NLPAPI)
	registerProxy(router, "/openapi/v1/auth", cfg.Gateway.AdminAPI)
	registerProxy(router, "/openapi/v1/asr", cfg.Gateway.ASRAPI)
	registerProxy(router, "/openapi/v1/meetings", cfg.Gateway.ASRAPI)
	registerProxy(router, "/openapi/v1/skills", cfg.Gateway.ASRAPI)
	registerProxy(router, "/openapi/v1/nlp", cfg.Gateway.NLPAPI)
	registerLegacyRoutes(router, cfg)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Info("gateway listening", zap.String("addr", addr))
	log.Fatal(router.Run(addr))
}

func registerLegacyRoutes(router *gin.Engine, cfg *pkgconfig.Config) {
	legacyASRExactPaths := []string{"/api/upload", "/api/recognize", "/api/recognize/vad", "/api/audio/to_summary", "/api/legacy/upload", "/api/legacy/recognize", "/api/legacy/recognize/vad", "/api/legacy/audio/to_summary"}
	legacyASRPrefixPaths := []string{"/api/task", "/api/legacy/task"}
	legacyNLPExactPaths := []string{"/api/meeting/summary", "/api/text/correct", "/api/templates", "/api/legacy/meeting/summary", "/api/legacy/text/correct", "/api/legacy/templates"}
	if cfg.Legacy.Enabled {
		for _, path := range legacyASRExactPaths {
			registerExactProxy(router, path, cfg.Gateway.ASRAPI)
		}
		for _, path := range legacyASRPrefixPaths {
			registerProxy(router, path, cfg.Gateway.ASRAPI)
		}
		for _, path := range legacyNLPExactPaths {
			registerExactProxy(router, path, cfg.Gateway.NLPAPI)
		}
		registerLegacyHealth(router)
		return
	}
	for _, path := range legacyASRExactPaths {
		registerExactGone(router, path)
	}
	for _, path := range legacyASRPrefixPaths {
		registerGone(router, path)
	}
	for _, path := range legacyNLPExactPaths {
		registerExactGone(router, path)
	}
	registerExactGone(router, "/api/health")
	registerExactGone(router, "/api/legacy/health")
}

func registerLegacyHealth(router *gin.Engine) {
	handler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"service":   "JushaAsr HTTP Server",
		})
	}
	router.GET("/api/health", handler)
	router.GET("/api/legacy/health", handler)
}

func registerGone(router *gin.Engine, prefix string) {
	handler := legacyDisabledHandler()
	router.Any(prefix, handler)
	router.Any(prefix+"/*path", handler)
}

func registerExactGone(router *gin.Engine, path string) {
	router.Any(path, legacyDisabledHandler())
}

func legacyDisabledHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusGone, gin.H{"success": false, "message": "legacy api disabled"})
	}
}

func legacyAccessLogger(path string) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()
		if !isLegacyPath(c.Request.URL.Path) || strings.TrimSpace(path) == "" {
			return
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return
		}
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer file.Close()
		_, _ = fmt.Fprintf(file, "%s method=%s path=%s status=%d latency_ms=%d ip=%s\n", time.Now().UTC().Format(time.RFC3339), c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(startedAt).Milliseconds(), c.ClientIP())
	}
}

func isLegacyPath(path string) bool {
	return strings.HasPrefix(path, "/api/legacy/") || path == "/api/upload" || path == "/api/recognize" || path == "/api/recognize/vad" || path == "/api/health" || path == "/api/meeting/summary" || path == "/api/text/correct" || path == "/api/templates" || path == "/api/audio/to_summary" || strings.HasPrefix(path, "/api/task/")
}

func registerProxy(router *gin.Engine, prefix, target string) {
	handler := newProxyHandler(target)
	router.Any(prefix, handler)
	router.Any(prefix+"/*path", handler)
}

func registerExactProxy(router *gin.Engine, path, target string) {
	router.Any(path, newProxyHandler(target))
}

func newProxyHandler(target string) gin.HandlerFunc {
	upstream, err := url.Parse(target)
	if err != nil {
		panic(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	// Strip CORS headers from upstream responses to avoid duplicates –
	// the gateway's own CORS middleware already sets them.
	proxy.ModifyResponse = func(resp *http.Response) error {
		for key := range resp.Header {
			if strings.HasPrefix(strings.ToLower(key), "access-control-") {
				resp.Header.Del(key)
			}
		}
		return nil
	}
	return func(c *gin.Context) {
		if isWebSocketUpgrade(c.Request) {
			proxyWebSocket(c, target)
			return
		}
		proxy.ServeHTTP(c.Writer, c.Request)
	}
}

func isWebSocketUpgrade(req *http.Request) bool {
	if req == nil {
		return false
	}
	return strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade") && strings.EqualFold(strings.TrimSpace(req.Header.Get("Upgrade")), "websocket")
}

func proxyWebSocket(c *gin.Context, target string) {
	upstreamURL := buildWSUpstreamURL(target, c.Request)

	upstreamConn, _, err := websocket.DefaultDialer.Dial(upstreamURL, nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream websocket unavailable"})
		return
	}
	defer upstreamConn.Close()

	clientConn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer clientConn.Close()

	errc := make(chan error, 2)
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

// registerWSProxy sets up a true WebSocket-level proxy that upgrades both
// the client connection and the upstream connection using gorilla/websocket,
// then forwards frames bidirectionally. This avoids the known limitations
// of httputil.ReverseProxy when proxying WebSocket through Gin.
func registerWSProxy(router *gin.Engine, prefix, target string, logger *zap.Logger) {
	handler := func(c *gin.Context) {
		upstreamURL := buildWSUpstreamURL(target, c.Request)

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
func buildWSUpstreamURL(target string, req *http.Request) string {
	parsed, _ := url.Parse(target)
	scheme := "ws"
	if parsed.Scheme == "https" {
		scheme = "wss"
	}

	path := req.URL.Path
	if strings.TrimSpace(parsed.Path) != "" && parsed.Path != "/" {
		path = strings.TrimRight(parsed.Path, "/") + req.URL.Path
	}
	if strings.TrimSpace(req.URL.RawQuery) != "" {
		return fmt.Sprintf("%s://%s%s?%s", scheme, parsed.Host, path, req.URL.RawQuery)
	}
	return fmt.Sprintf("%s://%s%s", scheme, parsed.Host, path)
}
