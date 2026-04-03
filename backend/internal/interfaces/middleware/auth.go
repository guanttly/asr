package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

const userClaimsKey = "auth_claims"

// Claims represents the JWT claims used by the backend apps.
type Claims struct {
	UserID uint64 `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken issues a signed JWT token.
func GenerateToken(secret string, expiresIn int64, userID uint64, role string) (string, error) {
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expiresIn) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// AuthRequired validates the Authorization header and stores claims in context.
func AuthRequired(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := tokenFromRequest(c)
		if tokenString == "" {
			response.Error(c, http.StatusUnauthorized, errcode.CodeUnauthorized, "missing auth token")
			c.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			response.Error(c, http.StatusUnauthorized, errcode.CodeUnauthorized, "invalid token")
			c.Abort()
			return
		}

		c.Set(userClaimsKey, claims)
		c.Next()
	}
}

func tokenFromRequest(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if header != "" {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
	}

	return strings.TrimSpace(c.Query("token"))
}

// UserIDFromContext extracts the authenticated user id.
func UserIDFromContext(c *gin.Context) uint64 {
	claims, ok := c.Get(userClaimsKey)
	if !ok {
		return 0
	}

	typedClaims, ok := claims.(*Claims)
	if !ok {
		return 0
	}

	return typedClaims.UserID
}

// RoleFromContext extracts the authenticated role.
func RoleFromContext(c *gin.Context) string {
	claims, ok := c.Get(userClaimsKey)
	if !ok {
		return ""
	}

	typedClaims, ok := claims.(*Claims)
	if !ok {
		return ""
	}

	return typedClaims.Role
}
