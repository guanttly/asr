package ws

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"go.uber.org/zap"
)

const (
	businessEventTypeEvent        = "event"
	businessEventTypeSubscribed   = "subscribed"
	businessEventTypeUnsubscribed = "unsubscribed"
	businessEventTypeError        = "error"
	businessEventTypePong         = "pong"
)

type BusinessEvent struct {
	Type      string    `json:"type"`
	Topic     string    `json:"topic,omitempty"`
	Business  string    `json:"business,omitempty"`
	Payload   any       `json:"payload,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type businessClientMessage struct {
	Type   string   `json:"type"`
	Topics []string `json:"topics,omitempty"`
}

type businessClient struct {
	userID uint64
	conn   *websocket.Conn
	send   chan []byte
	mu     sync.RWMutex
	closed bool
	topics map[string]struct{}
}

type BusinessHub struct {
	logger *zap.Logger

	mu      sync.RWMutex
	clients map[*businessClient]struct{}
}

func NewBusinessHub(logger *zap.Logger) *BusinessHub {
	return &BusinessHub{
		logger:  logger,
		clients: make(map[*businessClient]struct{}),
	}
}

func (h *BusinessHub) Handle(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &businessClient{
		userID: middleware.UserIDFromContext(c),
		conn:   conn,
		send:   make(chan []byte, 32),
		topics: make(map[string]struct{}),
	}

	h.addClient(client)
	defer h.removeClient(client)

	go h.writeLoop(client)
	h.enqueueJSON(client, BusinessEvent{
		Type:      businessEventTypeSubscribed,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"topics": []string{},
		},
	})

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var message businessClientMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			h.enqueueJSON(client, BusinessEvent{Type: businessEventTypeError, Timestamp: time.Now(), Payload: map[string]any{"message": "invalid websocket message"}})
			continue
		}

		switch strings.ToLower(strings.TrimSpace(message.Type)) {
		case "subscribe":
			topics := client.subscribe(message.Topics)
			h.enqueueJSON(client, BusinessEvent{Type: businessEventTypeSubscribed, Timestamp: time.Now(), Payload: map[string]any{"topics": topics}})
		case "unsubscribe":
			topics := client.unsubscribe(message.Topics)
			h.enqueueJSON(client, BusinessEvent{Type: businessEventTypeUnsubscribed, Timestamp: time.Now(), Payload: map[string]any{"topics": topics}})
		case "ping":
			h.enqueueJSON(client, BusinessEvent{Type: businessEventTypePong, Timestamp: time.Now()})
		default:
			h.enqueueJSON(client, BusinessEvent{Type: businessEventTypeError, Timestamp: time.Now(), Payload: map[string]any{"message": "unsupported websocket message type"}})
		}
	}
}

func (h *BusinessHub) PublishUserEvent(userID uint64, topic string, payload any) {
	if userID == 0 || strings.TrimSpace(topic) == "" {
		return
	}

	event := BusinessEvent{
		Type:      businessEventTypeEvent,
		Topic:     topic,
		Business:  businessFromTopic(topic),
		Payload:   payload,
		Timestamp: time.Now(),
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	clients := make([]*businessClient, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()

	for _, client := range clients {
		if client.userID != userID || !client.accepts(topic) {
			continue
		}
		if !client.enqueue(encoded) {
			if h.logger != nil {
				h.logger.Warn("dropping business event for slow websocket client", zap.Uint64("user_id", userID), zap.String("topic", topic))
			}
		}
	}
}

func (h *BusinessHub) addClient(client *businessClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client] = struct{}{}
}

func (h *BusinessHub) removeClient(client *businessClient) {
	h.mu.Lock()
	delete(h.clients, client)
	h.mu.Unlock()
	client.mu.Lock()
	if !client.closed {
		client.closed = true
		close(client.send)
	}
	client.mu.Unlock()
	_ = client.conn.Close()
}

func (h *BusinessHub) writeLoop(client *businessClient) {
	for payload := range client.send {
		if err := client.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			return
		}
	}
}

func (h *BusinessHub) enqueueJSON(client *businessClient, event BusinessEvent) {
	encoded, err := json.Marshal(event)
	if err != nil {
		return
	}
	if !client.enqueue(encoded) {
		if h.logger != nil {
			h.logger.Warn("dropping business websocket response for slow client", zap.Uint64("user_id", client.userID))
		}
	}
}

func (c *businessClient) enqueue(payload []byte) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- payload:
		return true
	default:
		return false
	}
}

func (c *businessClient) subscribe(topics []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		c.topics[topic] = struct{}{}
	}
	return c.snapshotTopicsLocked()
}

func (c *businessClient) unsubscribe(topics []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(topics) == 0 {
		c.topics = make(map[string]struct{})
		return nil
	}
	for _, topic := range topics {
		delete(c.topics, strings.TrimSpace(topic))
	}
	return c.snapshotTopicsLocked()
}

func (c *businessClient) accepts(topic string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, ok := c.topics[topic]; ok {
		return true
	}
	if _, ok := c.topics["*"]; ok {
		return true
	}
	parts := strings.Split(topic, ".")
	for i := 1; i < len(parts); i++ {
		wildcard := strings.Join(parts[:i], ".") + ".*"
		if _, ok := c.topics[wildcard]; ok {
			return true
		}
	}
	return false
}

func (c *businessClient) snapshotTopicsLocked() []string {
	topics := make([]string, 0, len(c.topics))
	for topic := range c.topics {
		topics = append(topics, topic)
	}
	return topics
}

func businessFromTopic(topic string) string {
	parts := strings.Split(strings.TrimSpace(topic), ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

var _ interface{ PublishUserEvent(uint64, string, any) } = (*BusinessHub)(nil)
