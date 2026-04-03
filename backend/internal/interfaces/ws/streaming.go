package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/lgt/asr/internal/infrastructure/asrengine"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// StreamingHandler bridges the browser WebSocket to the upstream ASR HTTP streaming API.
type StreamingHandler struct {
	engine            *asrengine.Client
	logger            *zap.Logger
	streamRolloverSec int
}

// StreamMessage is the normalized message format returned to the frontend.
type StreamMessage struct {
	Type          string `json:"type"`
	Text          string `json:"text"`
	IsFinal       bool   `json:"is_final"`
	Sequence      int    `json:"sequence"`
	ReceivedBytes int    `json:"received_bytes,omitempty"`
}

type clientControl struct {
	Type  string `json:"type"`
	Event string `json:"event"`
}

// NewStreamingHandler creates a websocket streaming handler.
func NewStreamingHandler(engine *asrengine.Client, logger *zap.Logger, streamRolloverSec int) *StreamingHandler {
	return &StreamingHandler{engine: engine, logger: logger, streamRolloverSec: streamRolloverSec}
}

// Handle upgrades the client connection and bridges audio frames to the upstream ASR
// HTTP streaming API (POST /api/start, /api/chunk, /api/finish).
func (h *StreamingHandler) Handle(c *gin.Context) {
	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer clientConn.Close()

	if h.engine == nil || h.engine.StreamURL() == "" {
		_ = h.writeMessage(clientConn, &sync.Mutex{}, StreamMessage{Type: "error", Text: "upstream asr service is not configured", IsFinal: true, Sequence: 1})
		return
	}

	bgCtx := context.Background()
	startSession := func() (string, time.Time, error) {
		ctx, cancel := context.WithTimeout(bgCtx, 10*time.Second)
		defer cancel()

		sessionID, err := h.engine.StartStreamSession(ctx)
		if err != nil {
			return "", time.Time{}, err
		}

		return sessionID, time.Now(), nil
	}
	finishSession := func(sessionID string) (*asrengine.StreamingSessionResult, error) {
		ctx, cancel := context.WithTimeout(bgCtx, 10*time.Second)
		defer cancel()
		return h.engine.FinishStreamSession(ctx, sessionID)
	}
	sendFinalResult := func(conn *websocket.Conn, mu *sync.Mutex, result *asrengine.StreamingSessionResult, sequence *int) {
		if result == nil || result.Text == "" {
			return
		}

		*sequence++
		_ = h.writeMessage(conn, mu, StreamMessage{
			Type:     "sentence",
			Text:     result.Text,
			IsFinal:  true,
			Sequence: *sequence,
		})
	}

	sessionID, sessionStartedAt, err := startSession()
	if err != nil {
		h.logger.Warn("failed to start upstream streaming session", zap.Error(err))
		_ = h.writeMessage(clientConn, &sync.Mutex{}, StreamMessage{Type: "error", Text: "failed to start upstream asr session", IsFinal: true, Sequence: 1})
		return
	}

	defer func() {
		if sessionID != "" {
			_, _ = finishSession(sessionID)
		}
	}()

	var writeMu sync.Mutex
	sequence := 0

	for {
		messageType, payload, err := clientConn.ReadMessage()
		if err != nil {
			return
		}

		// Handle JSON control messages from the frontend.
		if messageType == websocket.TextMessage {
			textPayload := bytes.TrimSpace(payload)
			var control clientControl
			if err := json.Unmarshal(textPayload, &control); err == nil && control.Type == "control" {
				if control.Event == "stop" {
					result, finishErr := finishSession(sessionID)
					sessionID = ""

					if finishErr == nil {
						sendFinalResult(clientConn, &writeMu, result, &sequence)
					}

					sequence++
					_ = h.writeMessage(clientConn, &writeMu, StreamMessage{
						Type: "ack", Text: "control:stop",
						IsFinal: true, Sequence: sequence,
					})
					return
				}

				sequence++
				_ = h.writeMessage(clientConn, &writeMu, StreamMessage{
					Type: "ack", Text: "control:" + control.Event,
					IsFinal: false, Sequence: sequence,
				})
				continue
			}
		}

		// Forward binary PCM audio chunks to upstream streaming API.
		if messageType == websocket.BinaryMessage && len(payload) > 0 {
			if h.streamRolloverSec > 0 && time.Since(sessionStartedAt) >= time.Duration(h.streamRolloverSec)*time.Second {
				result, finishErr := finishSession(sessionID)
				if finishErr != nil {
					h.logger.Warn("failed to rotate upstream streaming session", zap.Error(finishErr))
				}
				sendFinalResult(clientConn, &writeMu, result, &sequence)

				newSessionID, newSessionStartedAt, startErr := startSession()
				if startErr != nil {
					h.logger.Warn("failed to start rotated upstream streaming session", zap.Error(startErr))
					_ = h.writeMessage(clientConn, &writeMu, StreamMessage{Type: "error", Text: "failed to rotate upstream asr session", IsFinal: true, Sequence: sequence + 1})
					sessionID = ""
					return
				}

				sessionID = newSessionID
				sessionStartedAt = newSessionStartedAt
			}

			result, pushErr := h.engine.PushStreamChunk(bgCtx, sessionID, payload)
			if pushErr != nil {
				h.logger.Warn("failed to push chunk to upstream", zap.Error(pushErr), zap.Int("bytes", len(payload)))
				sequence++
				_ = h.writeMessage(clientConn, &writeMu, StreamMessage{Type: "error", Text: "failed to push audio chunk upstream", IsFinal: true, Sequence: sequence})
				return
			}

			sequence++
			_ = h.writeMessage(clientConn, &writeMu, StreamMessage{
				Type: "partial", Text: result.Text,
				IsFinal: false, Sequence: sequence,
				ReceivedBytes: len(payload),
			})
		}
	}
}

func (h *StreamingHandler) writeMessage(conn *websocket.Conn, mu *sync.Mutex, message StreamMessage) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()
	return conn.WriteMessage(websocket.TextMessage, payload)
}
