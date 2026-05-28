package middleware

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

const (
	defaultRateLimitRequestsPerMinute = 600
	defaultRateLimitBurst             = 120
	defaultRateLimitCleanupInterval   = time.Minute
)

type RateLimitConfig struct {
	RequestsPerMinute int
	Burst             int
	CleanupInterval   time.Duration
	KeyFunc           func(*gin.Context) string
	Skip              func(*gin.Context) bool
}

type rateLimitBucket struct {
	tokens float64
	seenAt time.Time
}

type rateLimiter struct {
	mu              sync.Mutex
	buckets         map[string]rateLimitBucket
	requestsPerSec  float64
	burst           float64
	cleanupInterval time.Duration
	lastCleanup     time.Time
}

func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerMinute: defaultRateLimitRequestsPerMinute,
		Burst:             defaultRateLimitBurst,
		CleanupInterval:   defaultRateLimitCleanupInterval,
	}
}

func RateLimit(config RateLimitConfig) gin.HandlerFunc {
	requestsPerMinute := config.RequestsPerMinute
	if requestsPerMinute <= 0 {
		requestsPerMinute = defaultRateLimitRequestsPerMinute
	}
	burst := config.Burst
	if burst <= 0 {
		burst = defaultRateLimitBurst
	}
	cleanupInterval := config.CleanupInterval
	if cleanupInterval <= 0 {
		cleanupInterval = defaultRateLimitCleanupInterval
	}

	limiter := &rateLimiter{
		buckets:         make(map[string]rateLimitBucket),
		requestsPerSec:  float64(requestsPerMinute) / 60,
		burst:           float64(burst),
		cleanupInterval: cleanupInterval,
		lastCleanup:     time.Now(),
	}

	keyFunc := config.KeyFunc
	if keyFunc == nil {
		keyFunc = func(ctx *gin.Context) string {
			return ctx.ClientIP()
		}
	}

	return func(ctx *gin.Context) {
		if config.Skip != nil && config.Skip(ctx) {
			ctx.Next()
			return
		}

		key := strings.TrimSpace(keyFunc(ctx))
		if key == "" {
			key = ctx.ClientIP()
		}

		allowed, retryAfter := limiter.allow(key, time.Now())
		if !allowed {
			ctx.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			if strings.HasPrefix(ctx.Request.URL.Path, "/openapi/") {
				response.OpenError(ctx, http.StatusTooManyRequests, errcode.OpenRateLimited, "rate limit exceeded")
			} else {
				response.Error(ctx, http.StatusTooManyRequests, errcode.CodeTooManyRequests, "rate limit exceeded")
			}
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

func (limiter *rateLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	limiter.cleanup(now)

	bucket, ok := limiter.buckets[key]
	if !ok {
		limiter.buckets[key] = rateLimitBucket{tokens: limiter.burst - 1, seenAt: now}
		return true, 0
	}

	elapsed := now.Sub(bucket.seenAt).Seconds()
	if elapsed > 0 {
		bucket.tokens = math.Min(limiter.burst, bucket.tokens+elapsed*limiter.requestsPerSec)
		bucket.seenAt = now
	}

	if bucket.tokens >= 1 {
		bucket.tokens--
		limiter.buckets[key] = bucket
		return true, 0
	}

	limiter.buckets[key] = bucket
	retrySeconds := math.Ceil((1 - bucket.tokens) / limiter.requestsPerSec)
	if retrySeconds < 1 {
		retrySeconds = 1
	}
	return false, time.Duration(retrySeconds) * time.Second
}

func (limiter *rateLimiter) cleanup(now time.Time) {
	if now.Sub(limiter.lastCleanup) < limiter.cleanupInterval {
		return
	}
	for key, bucket := range limiter.buckets {
		if now.Sub(bucket.seenAt) > limiter.cleanupInterval*2 {
			delete(limiter.buckets, key)
		}
	}
	limiter.lastCleanup = now
}
