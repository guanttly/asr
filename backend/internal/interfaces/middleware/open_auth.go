package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	appopenplatform "github.com/lgt/asr/internal/application/openplatform"
	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

const (
	openAuthClaimsKey = "open_auth_claims"
	openAuthAppKey    = "open_auth_app"
)

func OpenAuthRequired(service *appopenplatform.Service, capability string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, app, err := service.AuthenticateAccessToken(c.Request.Context(), openTokenFromRequest(c), capability)
		if err != nil {
			var rateLimitErr *appopenplatform.RateLimitError
			switch {
			case errors.Is(err, appopenplatform.ErrOpenAuthMissing):
				response.OpenError(c, http.StatusUnauthorized, errcode.OpenAuthMissing, "missing Authorization header")
			case errors.Is(err, appopenplatform.ErrOpenAuthInvalid):
				response.OpenError(c, http.StatusUnauthorized, errcode.OpenAuthInvalid, "invalid access token")
			case errors.Is(err, appopenplatform.ErrOpenAuthExpired):
				c.Header("Retry-After-Sec", "0")
				response.OpenError(c, http.StatusUnauthorized, errcode.OpenAuthExpired, "access token expired")
			case errors.Is(err, appopenplatform.ErrOpenAuthRevoked):
				response.OpenError(c, http.StatusUnauthorized, errcode.OpenAuthRevoked, "access token revoked")
			case errors.Is(err, appopenplatform.ErrOpenAppDisabled):
				response.OpenError(c, http.StatusForbidden, errcode.OpenAppDisabled, "application is disabled")
			case errors.Is(err, appopenplatform.ErrOpenCapDenied):
				response.OpenError(c, http.StatusForbidden, errcode.OpenCapDenied, "capability denied")
			case errors.As(err, &rateLimitErr):
				c.Header("Retry-After", strconv.Itoa(rateLimitErr.RetryAfterSec))
				response.OpenError(c, http.StatusTooManyRequests, errcode.OpenRateLimited, "rate limit exceeded")
			default:
				response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, err.Error())
			}
			c.Abort()
			return
		}
		c.Set(openAuthClaimsKey, claims)
		c.Set(openAuthAppKey, app)
		c.Next()
	}
}

func OpenClaimsFromContext(c *gin.Context) *appopenplatform.AccessTokenClaims {
	value, ok := c.Get(openAuthClaimsKey)
	if !ok {
		return nil
	}
	claims, _ := value.(*appopenplatform.AccessTokenClaims)
	return claims
}

func OpenAppFromContext(c *gin.Context) *openplatformdomain.App {
	value, ok := c.Get(openAuthAppKey)
	if !ok {
		return nil
	}
	app, _ := value.(*openplatformdomain.App)
	return app
}

func openTokenFromRequest(c *gin.Context) string {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if header != "" {
		if strings.HasPrefix(strings.ToLower(header), "bearer ") {
			return strings.TrimSpace(header[7:])
		}
		return header
	}
	return strings.TrimSpace(c.Query("access_token"))
}
