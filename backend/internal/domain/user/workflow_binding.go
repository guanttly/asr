package user

import "time"

// WorkflowBindings stores per-user default workflow bindings for frontend apps.
type WorkflowBindings struct {
	UserID             uint64    `json:"user_id"`
	RealtimeWorkflowID *uint64   `json:"realtime,omitempty"`
	BatchWorkflowID    *uint64   `json:"batch,omitempty"`
	MeetingWorkflowID  *uint64   `json:"meeting,omitempty"`
	VoiceWorkflowID    *uint64   `json:"voice_control,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
