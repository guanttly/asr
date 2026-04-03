package middleware

import "github.com/gin-gonic/gin"

// Recovery wraps gin's built-in recovery middleware.
func Recovery() gin.HandlerFunc {
	return gin.Recovery()
}
