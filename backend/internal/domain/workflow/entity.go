package workflow

import "time"

// Workflow represents a workflow template with ordered processing nodes.
type Workflow struct {
	ID                uint64             `json:"id"`
	Name              string             `json:"name"`
	Description       string             `json:"description"`
	WorkflowType      WorkflowType       `json:"workflow_type"`
	SourceKind        WorkflowSourceKind `json:"source_kind"`
	TargetKind        WorkflowTargetKind `json:"target_kind"`
	IsLegacy          bool               `json:"is_legacy"`
	ValidationMessage string             `json:"validation_message,omitempty"`
	OwnerType         OwnerType          `json:"owner_type"`
	OwnerID           uint64             `json:"owner_id"`
	SourceID          *uint64            `json:"source_id,omitempty"`
	IsPublished       bool               `json:"is_published"`
	Nodes             []Node             `json:"nodes,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

// Node represents an ordered processing step in a workflow.
type Node struct {
	ID         uint64    `json:"id"`
	WorkflowID uint64    `json:"workflow_id"`
	NodeType   NodeType  `json:"node_type"`
	Position   int       `json:"position"`
	Config     string    `json:"config"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// NodeDefault stores the default configuration for a node type.
type NodeDefault struct {
	NodeType  NodeType  `json:"node_type"`
	Config    string    `json:"config"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Execution records a single run of a workflow.
type Execution struct {
	ID           uint64          `json:"id"`
	WorkflowID   uint64          `json:"workflow_id"`
	TriggerType  TriggerType     `json:"trigger_type"`
	TriggerID    string          `json:"trigger_id"`
	InputText    string          `json:"input_text"`
	FinalText    string          `json:"final_text"`
	Status       ExecutionStatus `json:"status"`
	ErrorMessage string          `json:"error_message,omitempty"`
	NodeResults  []NodeResult    `json:"node_results,omitempty"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// CanTransition checks whether the execution can transition to the given status.
func (e *Execution) CanTransition(to ExecutionStatus) bool {
	switch e.Status {
	case ExecStatusPending:
		return to == ExecStatusRunning || to == ExecStatusFailed
	case ExecStatusRunning:
		return to == ExecStatusCompleted || to == ExecStatusFailed
	default:
		return false
	}
}

// NodeResult records the result of executing a single node.
type NodeResult struct {
	ID          uint64           `json:"id"`
	ExecutionID uint64           `json:"execution_id"`
	NodeID      uint64           `json:"node_id"`
	NodeType    NodeType         `json:"node_type"`
	Position    int              `json:"position"`
	InputText   string           `json:"input_text"`
	OutputText  string           `json:"output_text"`
	Status      NodeResultStatus `json:"status"`
	Detail      string           `json:"detail"`
	DurationMs  int              `json:"duration_ms"`
	ExecutedAt  *time.Time       `json:"executed_at,omitempty"`
}
