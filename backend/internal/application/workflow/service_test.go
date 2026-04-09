package workflow

import (
	"context"
	"testing"

	domain "github.com/lgt/asr/internal/domain/workflow"
)

type workflowRepoStub struct {
	created           *domain.Workflow
	items             []*domain.Workflow
	filteredItems     []*domain.Workflow
	listFilteredCalls int
}

func (r *workflowRepoStub) Create(_ context.Context, wf *domain.Workflow) error {
	wf.ID = 101
	r.created = wf
	return nil
}

func (r *workflowRepoStub) GetByID(_ context.Context, _ uint64) (*domain.Workflow, error) {
	panic("unexpected call to GetByID")
}

func (r *workflowRepoStub) Update(_ context.Context, _ *domain.Workflow) error {
	panic("unexpected call to Update")
}

func (r *workflowRepoStub) Delete(_ context.Context, _ uint64) error {
	panic("unexpected call to Delete")
}

func (r *workflowRepoStub) List(_ context.Context, _ *domain.OwnerType, _ *uint64, _ bool, _, _ int) ([]*domain.Workflow, int64, error) {
	return r.items, int64(len(r.items)), nil
}

func (r *workflowRepoStub) ListFiltered(_ context.Context, _ *domain.OwnerType, _ *uint64, _ bool, _ domain.WorkflowListFilter, offset, limit int) ([]*domain.Workflow, int64, error) {
	r.listFilteredCalls++
	items := append([]*domain.Workflow(nil), r.filteredItems...)
	total := int64(len(items))
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = len(items)
	}
	if offset >= len(items) {
		return []*domain.Workflow{}, total, nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], total, nil
}

type nodeRepoStub struct {
	items map[uint64][]domain.Node
}

func (r *nodeRepoStub) ListByWorkflow(_ context.Context, workflowID uint64) ([]domain.Node, error) {
	return r.items[workflowID], nil
}

func (r *nodeRepoStub) BatchSave(_ context.Context, _ uint64, _ []domain.Node) error {
	panic("unexpected call to BatchSave")
}

func (r *nodeRepoStub) DeleteByWorkflow(_ context.Context, _ uint64) error {
	panic("unexpected call to DeleteByWorkflow")
}

func TestCreateWorkflowPropagatesSourceID(t *testing.T) {
	repo := &workflowRepoStub{}
	service := NewService(repo, nil, nil, nil, nil)
	sourceID := uint64(42)

	result, err := service.CreateWorkflow(context.Background(), domain.OwnerUser, 7, &CreateWorkflowRequest{
		Name:        "派生工作流",
		Description: "从现有工作流另存为副本",
		SourceID:    &sourceID,
	})
	if err != nil {
		t.Fatalf("CreateWorkflow returned error: %v", err)
	}
	if repo.created == nil {
		t.Fatal("expected repository Create to receive workflow")
	}
	if repo.created.SourceID == nil || *repo.created.SourceID != sourceID {
		t.Fatalf("expected repository workflow source_id=%d, got %+v", sourceID, repo.created.SourceID)
	}
	if result.SourceID == nil || *result.SourceID != sourceID {
		t.Fatalf("expected response source_id=%d, got %+v", sourceID, result.SourceID)
	}
	if result.ID != 101 {
		t.Fatalf("expected response ID propagated from repository, got %d", result.ID)
	}
}

func TestListWorkflowsFilteredPaginatesAfterProfileFilter(t *testing.T) {
	batchType := domain.WorkflowTypeBatch
	repo := &workflowRepoStub{
		filteredItems: []*domain.Workflow{
			{ID: 2, Name: "batch-1", WorkflowType: domain.WorkflowTypeBatch, SourceKind: domain.SourceKindBatchASR, TargetKind: domain.TargetKindTranscript},
			{ID: 3, Name: "batch-2", WorkflowType: domain.WorkflowTypeBatch, SourceKind: domain.SourceKindBatchASR, TargetKind: domain.TargetKindTranscript},
		},
	}
	nodes := &nodeRepoStub{items: map[uint64][]domain.Node{
		2: {{WorkflowID: 2, NodeType: domain.NodeBatchASR, Position: 1, Enabled: true}},
		3: {{WorkflowID: 3, NodeType: domain.NodeBatchASR, Position: 1, Enabled: true}},
	}}
	service := NewService(repo, nodes, nil, nil, nil)

	result, err := service.ListWorkflowsFiltered(context.Background(), nil, nil, false, 0, 1, WorkflowListFilter{
		WorkflowType:  &batchType,
		IncludeLegacy: false,
	})
	if err != nil {
		t.Fatalf("ListWorkflowsFiltered returned error: %v", err)
	}
	if repo.listFilteredCalls != 1 {
		t.Fatalf("expected ListFiltered to be called once, got %d", repo.listFilteredCalls)
	}
	if result.Total != 2 {
		t.Fatalf("expected total from filtered repository=2, got %d", result.Total)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected repository-filtered paged items=1, got %d", len(result.Items))
	}
	if result.Items[0].ID != 2 {
		t.Fatalf("expected first repository-filtered workflow id=2, got %d", result.Items[0].ID)
	}
}

func TestBuildExecutionResumePlanStartsFromFailedNode(t *testing.T) {
	failedExec := &domain.Execution{
		ID:        30,
		InputText: "原始文本",
	}
	nodes := []domain.Node{
		{ID: 1, Position: 1, NodeType: domain.NodeFillerFilter},
		{ID: 2, Position: 2, NodeType: domain.NodeCustomRegex},
		{ID: 3, Position: 3, NodeType: domain.NodeLLMCorrection},
	}
	results := []domain.NodeResult{
		{NodeID: 1, Position: 1, NodeType: domain.NodeFillerFilter, InputText: "原始文本", OutputText: "过滤后文本", Status: domain.NodeResultSuccess},
		{NodeID: 2, Position: 2, NodeType: domain.NodeCustomRegex, InputText: "过滤后文本", OutputText: "", Status: domain.NodeResultFailed},
	}

	resumeNodes, carryResults, startInput, err := buildExecutionResumePlan(nodes, failedExec, results)
	if err != nil {
		t.Fatalf("buildExecutionResumePlan returned error: %v", err)
	}
	if len(resumeNodes) != 2 || resumeNodes[0].ID != 2 || resumeNodes[1].ID != 3 {
		t.Fatalf("expected resume nodes to start from failed node, got %+v", resumeNodes)
	}
	if len(carryResults) != 1 || carryResults[0].NodeID != 1 {
		t.Fatalf("expected successful node results to be preserved, got %+v", carryResults)
	}
	if startInput != "过滤后文本" {
		t.Fatalf("expected start input from failed node input, got %q", startInput)
	}
}
