package workflow

import (
	"encoding/json"
	"time"

	domain "github.com/lgt/asr/internal/domain/workflow"
)

// ─── Request DTOs ───────────────────────────────────────

// CreateWorkflowRequest is the request DTO for creating a workflow.
type CreateWorkflowRequest struct {
	Name        string            `json:"name" binding:"required"`
	Description string            `json:"description"`
	OwnerType   *domain.OwnerType `json:"owner_type,omitempty"`
	SourceID    *uint64           `json:"source_id,omitempty"`
}

// UpdateWorkflowRequest is the request DTO for updating workflow metadata.
type UpdateWorkflowRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	IsPublished *bool  `json:"is_published"`
}

// NodeRequest is the request DTO for a workflow node in batch operations.
type NodeRequest struct {
	NodeType string          `json:"node_type" binding:"required"`
	Position int             `json:"position"`
	Config   json.RawMessage `json:"config"`
	Enabled  *bool           `json:"enabled"`
}

// BatchUpdateNodesRequest is the request DTO for batch-updating workflow nodes.
type BatchUpdateNodesRequest struct {
	Nodes []NodeRequest `json:"nodes" binding:"required"`
}

// TestNodeRequest is the request DTO for testing a single node.
type TestNodeRequest struct {
	NodeType  string          `json:"node_type" binding:"required"`
	Config    json.RawMessage `json:"config" binding:"required"`
	InputText string          `json:"input_text" binding:"required"`
}

// ExecuteWorkflowRequest is the request DTO for executing a workflow.
type ExecuteWorkflowRequest struct {
	InputText string `json:"input_text" binding:"required"`
	AudioURL  string `json:"audio_url,omitempty"`
}

// ─── Response DTOs ──────────────────────────────────────

// WorkflowResponse is the response DTO for a workflow.
type WorkflowResponse struct {
	ID                uint64                    `json:"id"`
	Name              string                    `json:"name"`
	Description       string                    `json:"description"`
	WorkflowType      domain.WorkflowType       `json:"workflow_type"`
	SourceKind        domain.WorkflowSourceKind `json:"source_kind"`
	TargetKind        domain.WorkflowTargetKind `json:"target_kind"`
	IsLegacy          bool                      `json:"is_legacy"`
	ValidationMessage string                    `json:"validation_message,omitempty"`
	OwnerType         domain.OwnerType          `json:"owner_type"`
	OwnerID           uint64                    `json:"owner_id"`
	SourceID          *uint64                   `json:"source_id,omitempty"`
	IsPublished       bool                      `json:"is_published"`
	Nodes             []NodeResponse            `json:"nodes,omitempty"`
	CreatedAt         time.Time                 `json:"created_at"`
	UpdatedAt         time.Time                 `json:"updated_at"`
}

// NodeResponse is the response DTO for a workflow node.
type NodeResponse struct {
	ID       uint64          `json:"id"`
	NodeType domain.NodeType `json:"node_type"`
	Label    string          `json:"label"`
	Position int             `json:"position"`
	Config   json.RawMessage `json:"config"`
	Enabled  bool            `json:"enabled"`
}

// WorkflowListResponse wraps a paginated list of workflows.
type WorkflowListResponse struct {
	Items []*WorkflowResponse `json:"items"`
	Total int64               `json:"total"`
}

// ExecutionResponse is the response DTO for a workflow execution.
type ExecutionResponse struct {
	ID           uint64                 `json:"id"`
	WorkflowID   uint64                 `json:"workflow_id"`
	TriggerType  domain.TriggerType     `json:"trigger_type"`
	TriggerID    string                 `json:"trigger_id"`
	InputText    string                 `json:"input_text"`
	FinalText    string                 `json:"final_text"`
	Status       domain.ExecutionStatus `json:"status"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	NodeResults  []NodeResultResponse   `json:"node_results,omitempty"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}

// NodeResultResponse is the response DTO for a single node execution result.
type NodeResultResponse struct {
	ID         uint64                  `json:"id"`
	NodeID     uint64                  `json:"node_id"`
	NodeType   domain.NodeType         `json:"node_type"`
	Label      string                  `json:"label"`
	Position   int                     `json:"position"`
	InputText  string                  `json:"input_text"`
	OutputText string                  `json:"output_text"`
	Status     domain.NodeResultStatus `json:"status"`
	Detail     json.RawMessage         `json:"detail,omitempty"`
	DurationMs int                     `json:"duration_ms"`
	ExecutedAt *time.Time              `json:"executed_at,omitempty"`
}

// TestNodeResponse is the response DTO for a single node test.
type TestNodeResponse struct {
	OutputText string          `json:"output_text"`
	Detail     json.RawMessage `json:"detail,omitempty"`
	DurationMs int             `json:"duration_ms"`
	Error      string          `json:"error,omitempty"`
}

// NodeTypeInfo describes a registered node type.
type NodeTypeInfo struct {
	Type        domain.NodeType `json:"type"`
	Label       string          `json:"label"`
	Role        string          `json:"role"`
	Description string          `json:"description,omitempty"`
}

// ─── Conversion Helpers ─────────────────────────────────

// ToWorkflowResponse converts a domain Workflow to response DTO.
func ToWorkflowResponse(wf *domain.Workflow) *WorkflowResponse {
	resp := &WorkflowResponse{
		ID:                wf.ID,
		Name:              wf.Name,
		Description:       wf.Description,
		WorkflowType:      wf.WorkflowType,
		SourceKind:        wf.SourceKind,
		TargetKind:        wf.TargetKind,
		IsLegacy:          wf.IsLegacy,
		ValidationMessage: wf.ValidationMessage,
		OwnerType:         wf.OwnerType,
		OwnerID:           wf.OwnerID,
		SourceID:          wf.SourceID,
		IsPublished:       wf.IsPublished,
		CreatedAt:         wf.CreatedAt,
		UpdatedAt:         wf.UpdatedAt,
	}
	for _, n := range wf.Nodes {
		resp.Nodes = append(resp.Nodes, toNodeResponse(n))
	}
	return resp
}

func toNodeResponse(n domain.Node) NodeResponse {
	cfg := json.RawMessage(n.Config)
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	return NodeResponse{
		ID:       n.ID,
		NodeType: n.NodeType,
		Label:    n.NodeType.Label(),
		Position: n.Position,
		Config:   cfg,
		Enabled:  n.Enabled,
	}
}

// ToExecutionResponse converts a domain Execution to response DTO.
func ToExecutionResponse(exec *domain.Execution) *ExecutionResponse {
	resp := &ExecutionResponse{
		ID:           exec.ID,
		WorkflowID:   exec.WorkflowID,
		TriggerType:  exec.TriggerType,
		TriggerID:    exec.TriggerID,
		InputText:    exec.InputText,
		FinalText:    exec.FinalText,
		Status:       exec.Status,
		ErrorMessage: exec.ErrorMessage,
		StartedAt:    exec.StartedAt,
		CompletedAt:  exec.CompletedAt,
		CreatedAt:    exec.CreatedAt,
	}
	for _, nr := range exec.NodeResults {
		resp.NodeResults = append(resp.NodeResults, toNodeResultResponse(nr))
	}
	return resp
}

func toNodeResultResponse(nr domain.NodeResult) NodeResultResponse {
	detail := json.RawMessage(nr.Detail)
	if len(detail) == 0 {
		detail = nil
	}
	return NodeResultResponse{
		ID:         nr.ID,
		NodeID:     nr.NodeID,
		NodeType:   nr.NodeType,
		Label:      nr.NodeType.Label(),
		Position:   nr.Position,
		InputText:  nr.InputText,
		OutputText: nr.OutputText,
		Status:     nr.Status,
		Detail:     detail,
		DurationMs: nr.DurationMs,
		ExecutedAt: nr.ExecutedAt,
	}
}
