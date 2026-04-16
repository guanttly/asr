package workflow

import "context"

type WorkflowListFilter struct {
	WorkflowType  *WorkflowType
	SourceKind    *WorkflowSourceKind
	TargetKind    *WorkflowTargetKind
	IncludeLegacy bool
}

// WorkflowRepository manages workflow CRUD operations.
type WorkflowRepository interface {
	Create(ctx context.Context, wf *Workflow) error
	GetByID(ctx context.Context, id uint64) (*Workflow, error)
	Update(ctx context.Context, wf *Workflow) error
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context, ownerType *OwnerType, ownerID *uint64, publishedOnly bool, offset, limit int) ([]*Workflow, int64, error)
	ListFiltered(ctx context.Context, ownerType *OwnerType, ownerID *uint64, publishedOnly bool, filter WorkflowListFilter, offset, limit int) ([]*Workflow, int64, error)
}

// NodeRepository manages workflow node CRUD operations.
type NodeRepository interface {
	ListByWorkflow(ctx context.Context, workflowID uint64) ([]Node, error)
	BatchSave(ctx context.Context, workflowID uint64, nodes []Node) error
	DeleteByWorkflow(ctx context.Context, workflowID uint64) error
}

// NodeDefaultRepository manages node-type level default configurations.
type NodeDefaultRepository interface {
	List(ctx context.Context) ([]NodeDefault, error)
	GetByType(ctx context.Context, nodeType NodeType) (*NodeDefault, error)
	Upsert(ctx context.Context, item *NodeDefault) error
}

// ExecutionRepository manages workflow execution records.
type ExecutionRepository interface {
	Create(ctx context.Context, exec *Execution) error
	GetByID(ctx context.Context, id uint64) (*Execution, error)
	Update(ctx context.Context, exec *Execution) error
	ListByWorkflow(ctx context.Context, workflowID uint64, offset, limit int) ([]*Execution, int64, error)
	ListByTrigger(ctx context.Context, triggerType TriggerType, triggerID string, offset, limit int) ([]*Execution, int64, error)
}

// NodeResultRepository manages node-level execution results.
type NodeResultRepository interface {
	BatchCreate(ctx context.Context, results []NodeResult) error
	ListByExecution(ctx context.Context, executionID uint64) ([]NodeResult, error)
}
