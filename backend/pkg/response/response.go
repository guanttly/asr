package response

import "github.com/gin-gonic/gin"

// Envelope is the standard API response shape.
type Envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Success sends a success response.
func Success(c *gin.Context, data any) {
	c.JSON(200, Envelope{Code: 0, Message: "ok", Data: data})
}

// Error sends an error response.
func Error(c *gin.Context, statusCode int, code int, message string) {
	c.JSON(statusCode, Envelope{Code: code, Message: message})
}
