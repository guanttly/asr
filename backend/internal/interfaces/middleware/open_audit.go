package middleware

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
)

const openRequestIDKey = "open_request_id"

type captureResponseWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *captureResponseWriter) Write(data []byte) (int, error) {
	_, _ = w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

func (w *captureResponseWriter) WriteString(data string) (int, error) {
	w.body.WriteString(data)
	return w.ResponseWriter.WriteString(data)
}

func OpenAPIAudit(callLogRepo openplatformdomain.CallLogRepository, capability string) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateOpenRequestID()
		}
		c.Set(openRequestIDKey, requestID)
		c.Header("X-Request-ID", requestID)

		writer := &captureResponseWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		startedAt := time.Now()

		c.Next()

		app := OpenAppFromContext(c)
		if app == nil || callLogRepo == nil {
			return
		}

		statusCode := c.Writer.Status()
		if statusCode == 0 {
			statusCode = 200
		}
		latencyMs := time.Since(startedAt).Milliseconds()
		if latencyMs < 0 {
			latencyMs = 0
		}

		_ = callLogRepo.Create(context.Background(), &openplatformdomain.CallLog{
			RequestID:  requestID,
			AppID:      app.ID,
			Capability: capability,
			Route:      auditRoute(c),
			HTTPStatus: uint16(statusCode),
			ErrCode:    extractOpenErrCode(writer.body.Bytes()),
			LatencyMs:  uint32(latencyMs),
			IP:         c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
		})
	}
}

func RequestIDFromContext(c *gin.Context) string {
	value, ok := c.Get(openRequestIDKey)
	if !ok {
		return ""
	}
	requestID, _ := value.(string)
	return requestID
}

func auditRoute(c *gin.Context) string {
	if fullPath := c.FullPath(); fullPath != "" {
		return fullPath
	}
	if c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return ""
}

func extractOpenErrCode(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var body struct {
		Code any `json:"code"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return ""
	}
	code, _ := body.Code.(string)
	return code
}

func generateOpenRequestID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err == nil {
		return "req_" + base64.RawURLEncoding.EncodeToString(buf)
	}
	return "req_" + time.Now().Format("20060102150405.000000000")
}
