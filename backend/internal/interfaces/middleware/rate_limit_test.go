package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimitRejectsRequestsOverBurst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(RateLimitConfig{RequestsPerMinute: 60, Burst: 2}))
	router.GET("/api/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"ok": true})
	})

	for requestIndex := 0; requestIndex < 2; requestIndex++ {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/ping", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected request %d to pass, got %d", requestIndex+1, recorder.Code)
		}
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/ping", nil))
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected rate limited request to return 429, got %d", recorder.Code)
	}
	if recorder.Header().Get("Retry-After") != "1" {
		t.Fatalf("expected Retry-After header in seconds, got %q", recorder.Header().Get("Retry-After"))
	}
}

func TestRateLimitUsesOpenAPIEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimit(RateLimitConfig{RequestsPerMinute: 60, Burst: 1}))
	router.GET("/openapi/v1/asr/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"ok": true})
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/openapi/v1/asr/ping", nil))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/openapi/v1/asr/ping", nil))
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected openapi request to return 429, got %d", recorder.Code)
	}
	if body := recorder.Body.String(); body == "" || !strings.Contains(body, "ERR_OPEN_RATE_LIMITED") {
		t.Fatalf("expected openapi rate limit code, got %s", body)
	}
}

func TestRateLimitRefillsTokens(t *testing.T) {
	limiter := &rateLimiter{
		buckets:         make(map[string]rateLimitBucket),
		requestsPerSec:  1,
		burst:           1,
		cleanupInterval: time.Minute,
		lastCleanup:     time.Unix(0, 0),
	}
	now := time.Unix(100, 0)
	if ok, _ := limiter.allow("client", now); !ok {
		t.Fatal("expected first request to pass")
	}
	if ok, _ := limiter.allow("client", now); ok {
		t.Fatal("expected immediate second request to be limited")
	}
	if ok, _ := limiter.allow("client", now.Add(time.Second)); !ok {
		t.Fatal("expected request to pass after refill")
	}
}
