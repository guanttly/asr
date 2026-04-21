package workflow

import (
	"context"
	"encoding/json"
	"testing"

	domain "github.com/lgt/asr/internal/domain/workflow"
	wfengine "github.com/lgt/asr/internal/infrastructure/workflow"
	"go.uber.org/zap"
)

type workflowRepoStub struct {
	created           *domain.Workflow
	items             []*domain.Workflow
	filteredItems     []*domain.Workflow
	listFilteredCalls int
	byID              map[uint64]*domain.Workflow
	updated           *domain.Workflow
}

func (r *workflowRepoStub) Create(_ context.Context, wf *domain.Workflow) error {
	wf.ID = 101
	if r.byID == nil {
		r.byID = map[uint64]*domain.Workflow{}
	}
	copyItem := *wf
	r.byID[wf.ID] = &copyItem
	r.created = wf
	return nil
}

func (r *workflowRepoStub) GetByID(_ context.Context, id uint64) (*domain.Workflow, error) {
	if item, ok := r.byID[id]; ok {
		copyItem := *item
		return &copyItem, nil
	}
	panic("unexpected call to GetByID")
}

func (r *workflowRepoStub) Update(_ context.Context, wf *domain.Workflow) error {
	r.updated = wf
	if r.byID != nil {
		copyItem := *wf
		r.byID[wf.ID] = &copyItem
	}
	return nil
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
	items          map[uint64][]domain.Node
	lastSaved      []domain.Node
	lastWorkflowID uint64
}

type nodeDefaultRepoStub struct {
	items    map[domain.NodeType]domain.NodeDefault
	upserted *domain.NodeDefault
}

func (r *nodeDefaultRepoStub) List(_ context.Context) ([]domain.NodeDefault, error) {
	items := make([]domain.NodeDefault, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	return items, nil
}

func (r *nodeDefaultRepoStub) GetByType(_ context.Context, nodeType domain.NodeType) (*domain.NodeDefault, error) {
	item, ok := r.items[nodeType]
	if !ok {
		return nil, nil
	}
	copyItem := item
	return &copyItem, nil
}

func (r *nodeDefaultRepoStub) Upsert(_ context.Context, item *domain.NodeDefault) error {
	if r.items == nil {
		r.items = map[domain.NodeType]domain.NodeDefault{}
	}
	r.items[item.NodeType] = *item
	r.upserted = item
	return nil
}

type captureNodeHandler struct {
	lastConfig map[string]any
}

func (h *captureNodeHandler) Validate(config json.RawMessage) error {
	return json.Unmarshal(config, &h.lastConfig)
}

func (h *captureNodeHandler) Execute(_ context.Context, config json.RawMessage, inputText string, _ *wfengine.ExecutionMeta) (string, json.RawMessage, error) {
	if err := json.Unmarshal(config, &h.lastConfig); err != nil {
		return inputText, nil, err
	}
	return inputText, nil, nil
}

func (r *nodeRepoStub) ListByWorkflow(_ context.Context, workflowID uint64) ([]domain.Node, error) {
	return r.items[workflowID], nil
}

func (r *nodeRepoStub) BatchSave(_ context.Context, workflowID uint64, nodes []domain.Node) error {
	r.lastWorkflowID = workflowID
	r.lastSaved = append([]domain.Node(nil), nodes...)
	if r.items == nil {
		r.items = map[uint64][]domain.Node{}
	}
	r.items[workflowID] = append([]domain.Node(nil), nodes...)
	return nil
}

func (r *nodeRepoStub) DeleteByWorkflow(_ context.Context, _ uint64) error {
	panic("unexpected call to DeleteByWorkflow")
}

func TestCreateWorkflowPropagatesSourceID(t *testing.T) {
	repo := &workflowRepoStub{}
	nodes := &nodeRepoStub{items: map[uint64][]domain.Node{}}
	service := NewService(repo, nodes, nil, nil, nil, nil)
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

func TestCreateWorkflowSeedsFixedBoundaryNodes(t *testing.T) {
	repo := &workflowRepoStub{}
	nodes := &nodeRepoStub{items: map[uint64][]domain.Node{}}
	service := NewService(repo, nodes, nil, nil, nil, nil)
	workflowType := domain.WorkflowTypeMeeting

	result, err := service.CreateWorkflow(context.Background(), domain.OwnerUser, 7, &CreateWorkflowRequest{
		Name:         "会议纪要工作流",
		Description:  "自动固化会议纪要边界节点",
		WorkflowType: &workflowType,
	})
	if err != nil {
		t.Fatalf("CreateWorkflow returned error: %v", err)
	}
	if len(nodes.lastSaved) != 2 {
		t.Fatalf("expected two fixed nodes to be seeded, got %d", len(nodes.lastSaved))
	}
	if nodes.lastSaved[0].NodeType != domain.NodeBatchASR || nodes.lastSaved[0].Position != 1 || !nodes.lastSaved[0].Enabled {
		t.Fatalf("expected first fixed source node to be batch_asr at position 1, got %+v", nodes.lastSaved[0])
	}
	if nodes.lastSaved[1].NodeType != domain.NodeMeetingSummary || nodes.lastSaved[1].Position != 2 || !nodes.lastSaved[1].Enabled {
		t.Fatalf("expected fixed sink node to be meeting_summary at position 2, got %+v", nodes.lastSaved[1])
	}
	if result.WorkflowType != domain.WorkflowTypeMeeting || result.SourceKind != domain.SourceKindBatchASR || result.TargetKind != domain.TargetKindMeetingSummary {
		t.Fatalf("expected meeting profile in response, got %+v", result)
	}
	if len(result.Nodes) != 2 || !result.Nodes[0].IsFixed || !result.Nodes[1].IsFixed {
		t.Fatalf("expected fixed node metadata in response, got %+v", result.Nodes)
	}
}

func TestCreateVoiceControlWorkflowSeedsIntentSink(t *testing.T) {
	repo := &workflowRepoStub{}
	nodes := &nodeRepoStub{items: map[uint64][]domain.Node{}}
	service := NewService(repo, nodes, nil, nil, nil, nil)
	workflowType := domain.WorkflowTypeVoice

	result, err := service.CreateWorkflow(context.Background(), domain.OwnerUser, 7, &CreateWorkflowRequest{
		Name:         "语音控制工作流",
		Description:  "识别唤醒后的控制指令",
		WorkflowType: &workflowType,
	})
	if err != nil {
		t.Fatalf("CreateWorkflow returned error: %v", err)
	}
	if len(nodes.lastSaved) != 2 {
		t.Fatalf("expected wake source and intent sink nodes, got %d", len(nodes.lastSaved))
	}
	if nodes.lastSaved[0].NodeType != domain.NodeVoiceWake || nodes.lastSaved[0].Position != 1 || !nodes.lastSaved[0].Enabled {
		t.Fatalf("expected fixed source node to be voice_wake at position 1, got %+v", nodes.lastSaved[0])
	}
	if nodes.lastSaved[1].NodeType != domain.NodeVoiceIntent || nodes.lastSaved[1].Position != 2 || !nodes.lastSaved[1].Enabled {
		t.Fatalf("expected fixed sink node to be voice_intent at position 2, got %+v", nodes.lastSaved[1])
	}
	if result.WorkflowType != domain.WorkflowTypeVoice || result.SourceKind != domain.SourceKindVoiceWake || result.TargetKind != domain.TargetKindVoiceCommand {
		t.Fatalf("expected voice control profile in response, got %+v", result)
	}
	if len(result.Nodes) != 2 || !result.Nodes[0].IsFixed || !result.Nodes[1].IsFixed {
		t.Fatalf("expected fixed node metadata in response, got %+v", result.Nodes)
	}
}

func TestBatchUpdateNodesRejectsChangingFixedBoundary(t *testing.T) {
	repo := &workflowRepoStub{byID: map[uint64]*domain.Workflow{
		101: {
			ID:           101,
			WorkflowType: domain.WorkflowTypeMeeting,
			SourceKind:   domain.SourceKindBatchASR,
			TargetKind:   domain.TargetKindMeetingSummary,
		},
	}}
	nodes := &nodeRepoStub{items: map[uint64][]domain.Node{
		101: {
			{WorkflowID: 101, NodeType: domain.NodeBatchASR, Position: 1, Enabled: true},
			{WorkflowID: 101, NodeType: domain.NodeMeetingSummary, Position: 2, Enabled: true},
		},
	}}
	service := NewService(repo, nodes, nil, nil, nil, nil)

	_, err := service.BatchUpdateNodes(context.Background(), 101, &BatchUpdateNodesRequest{
		Nodes: []NodeRequest{
			{NodeType: string(domain.NodeMeetingSummary), Position: 1, Enabled: boolPtr(true), Config: json.RawMessage(`{}`)},
			{NodeType: string(domain.NodeBatchASR), Position: 2, Enabled: boolPtr(true), Config: json.RawMessage(`{}`)},
		},
	})
	if err == nil {
		t.Fatal("expected BatchUpdateNodes to reject reordered fixed nodes")
	}
	if nodes.lastSaved != nil {
		t.Fatalf("expected BatchSave not to be called on invalid boundary update, got %+v", nodes.lastSaved)
	}
	if err.Error() != "固化源节点必须保持在第一位" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBatchUpdateNodesRejectsWorkflowTypeChange(t *testing.T) {
	repo := &workflowRepoStub{byID: map[uint64]*domain.Workflow{
		101: {
			ID:           101,
			WorkflowType: domain.WorkflowTypeBatch,
			SourceKind:   domain.SourceKindBatchASR,
			TargetKind:   domain.TargetKindTranscript,
		},
	}}
	nodes := &nodeRepoStub{items: map[uint64][]domain.Node{
		101: {
			{WorkflowID: 101, NodeType: domain.NodeBatchASR, Position: 1, Enabled: true},
			{WorkflowID: 101, NodeType: domain.NodeFillerFilter, Position: 2, Enabled: true},
		},
	}}
	service := NewService(repo, nodes, nil, nil, nil, nil)

	_, err := service.BatchUpdateNodes(context.Background(), 101, &BatchUpdateNodesRequest{
		Nodes: []NodeRequest{
			{NodeType: string(domain.NodeBatchASR), Position: 1, Enabled: boolPtr(true), Config: json.RawMessage(`{}`)},
			{NodeType: string(domain.NodeFillerFilter), Position: 2, Enabled: boolPtr(true), Config: json.RawMessage(`{}`)},
			{NodeType: string(domain.NodeMeetingSummary), Position: 3, Enabled: boolPtr(true), Config: json.RawMessage(`{}`)},
		},
	})
	if err == nil {
		t.Fatal("expected BatchUpdateNodes to reject workflow type change")
	}
	if err.Error() != "工作流类型在创建时已确定，当前仅允许编辑节点配置和中间处理链路，不能改变入口/出口场景" {
		t.Fatalf("unexpected error: %v", err)
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
	service := NewService(repo, nodes, nil, nil, nil, nil)

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

func TestTestNodeMergesGlobalDefaults(t *testing.T) {
	defaultRepo := &nodeDefaultRepoStub{items: map[domain.NodeType]domain.NodeDefault{
		domain.NodeLLMCorrection: {
			NodeType: domain.NodeLLMCorrection,
			Config:   `{"endpoint":"http://llm.internal","model":"qwen-default","max_tokens":2048}`,
		},
	}}
	handler := &captureNodeHandler{}
	engine := wfengine.NewEngine(zap.NewNop())
	engine.RegisterHandler(domain.NodeLLMCorrection, handler)
	service := NewService(nil, nil, defaultRepo, nil, nil, engine)

	_, err := service.TestNode(context.Background(), &TestNodeRequest{
		NodeType:  string(domain.NodeLLMCorrection),
		Config:    json.RawMessage(`{"prompt_template":"请纠正：{{TEXT}}"}`),
		InputText: "测试文本",
	})
	if err != nil {
		t.Fatalf("TestNode returned error: %v", err)
	}
	if handler.lastConfig["endpoint"] != "http://llm.internal" {
		t.Fatalf("expected merged endpoint from defaults, got %+v", handler.lastConfig["endpoint"])
	}
	if handler.lastConfig["model"] != "qwen-default" {
		t.Fatalf("expected merged model from defaults, got %+v", handler.lastConfig["model"])
	}
	if handler.lastConfig["prompt_template"] != "请纠正：{{TEXT}}" {
		t.Fatalf("expected request prompt_template to override defaults, got %+v", handler.lastConfig["prompt_template"])
	}
	if handler.lastConfig["max_tokens"] != float64(2048) {
		t.Fatalf("expected default max_tokens to be preserved, got %+v", handler.lastConfig["max_tokens"])
	}
}

func TestUpdateNodeDefaultReturnsEffectiveConfig(t *testing.T) {
	defaultRepo := &nodeDefaultRepoStub{}
	engine := wfengine.NewEngine(zap.NewNop())
	engine.RegisterHandler(domain.NodeLLMCorrection, &captureNodeHandler{})
	service := NewService(nil, nil, defaultRepo, nil, nil, engine)

	result, err := service.UpdateNodeDefault(context.Background(), domain.NodeLLMCorrection, &UpdateNodeDefaultRequest{
		Config: json.RawMessage(`{"endpoint":"http://llm.internal","model":"qwen-plus"}`),
	})
	if err != nil {
		t.Fatalf("UpdateNodeDefault returned error: %v", err)
	}
	if defaultRepo.upserted == nil {
		t.Fatal("expected default repo Upsert to be called")
	}
	if defaultRepo.upserted.Config != `{"endpoint":"http://llm.internal","model":"qwen-plus"}` {
		t.Fatalf("expected raw override config to be persisted, got %s", defaultRepo.upserted.Config)
	}

	var payload map[string]any
	if err := json.Unmarshal(result.DefaultConfig, &payload); err != nil {
		t.Fatalf("unmarshal effective default config: %v", err)
	}
	if payload["endpoint"] != "http://llm.internal" || payload["model"] != "qwen-plus" {
		t.Fatalf("expected effective default config to include stored override, got %+v", payload)
	}
	if payload["temperature"] != 0.3 {
		t.Fatalf("expected builtin default temperature to be preserved, got %+v", payload["temperature"])
	}
}

func boolPtr(value bool) *bool {
	return &value
}
