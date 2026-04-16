package workflow

import (
	"context"
	"encoding/json"
)

type NodeStreamEventType string

const (
	NodeStreamEventStatus NodeStreamEventType = "status"
	NodeStreamEventDelta  NodeStreamEventType = "delta"
	NodeStreamEventDone   NodeStreamEventType = "done"
)

type NodeStreamEvent struct {
	Type       NodeStreamEventType `json:"type"`
	Message    string              `json:"message,omitempty"`
	Delta      string              `json:"delta,omitempty"`
	OutputText string              `json:"output_text,omitempty"`
	Detail     json.RawMessage     `json:"detail,omitempty"`
	DurationMs int                 `json:"duration_ms,omitempty"`
	Error      string              `json:"error,omitempty"`
}

type StreamEmitter func(event *NodeStreamEvent) error

// NodeHandler is the interface that every workflow node processor must implement.
type NodeHandler interface {
	// Execute processes input text and returns output text with optional detail JSON.
	Execute(ctx context.Context, config json.RawMessage, inputText string, meta *ExecutionMeta) (outputText string, detail json.RawMessage, err error)

	// Validate checks whether the given config is valid for this node type.
	Validate(config json.RawMessage) error
}

// StreamingNodeHandler can emit incremental node output while executing.
type StreamingNodeHandler interface {
	NodeHandler
	ExecuteStream(ctx context.Context, config json.RawMessage, inputText string, meta *ExecutionMeta, emit StreamEmitter) (outputText string, detail json.RawMessage, err error)
}

// ExecutionMeta carries contextual information about the current workflow execution.
// Node handlers can use this for context-dependent processing (e.g., accessing audio URL).
type ExecutionMeta struct {
	AudioURL      string `json:"audio_url,omitempty"`
	AudioFilePath string `json:"-"`
	TaskID        uint64 `json:"task_id,omitempty"`
	UserID        uint64 `json:"user_id,omitempty"`
	MeetingID     uint64 `json:"meeting_id,omitempty"`
}
