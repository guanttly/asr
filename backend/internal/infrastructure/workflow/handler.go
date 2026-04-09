package workflow

import (
	"context"
	"encoding/json"
)

// NodeHandler is the interface that every workflow node processor must implement.
type NodeHandler interface {
	// Execute processes input text and returns output text with optional detail JSON.
	Execute(ctx context.Context, config json.RawMessage, inputText string, meta *ExecutionMeta) (outputText string, detail json.RawMessage, err error)

	// Validate checks whether the given config is valid for this node type.
	Validate(config json.RawMessage) error
}

// ExecutionMeta carries contextual information about the current workflow execution.
// Node handlers can use this for context-dependent processing (e.g., accessing audio URL).
type ExecutionMeta struct {
	AudioURL  string `json:"audio_url,omitempty"`
	TaskID    uint64 `json:"task_id,omitempty"`
	UserID    uint64 `json:"user_id,omitempty"`
	MeetingID uint64 `json:"meeting_id,omitempty"`
}
