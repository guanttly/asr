package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	domain "github.com/lgt/asr/internal/domain/workflow"
	engine "github.com/lgt/asr/internal/infrastructure/workflow"
)

// Service provides workflow management and execution operations.
type Service struct {
	workflowRepo    domain.WorkflowRepository
	nodeRepo        domain.NodeRepository
	nodeDefaultRepo domain.NodeDefaultRepository
	executionRepo   domain.ExecutionRepository
	nodeResultRepo  domain.NodeResultRepository
	engine          *engine.Engine
}

// NewService creates a new workflow application service.
func NewService(
	workflowRepo domain.WorkflowRepository,
	nodeRepo domain.NodeRepository,
	nodeDefaultRepo domain.NodeDefaultRepository,
	executionRepo domain.ExecutionRepository,
	nodeResultRepo domain.NodeResultRepository,
	eng *engine.Engine,
) *Service {
	return &Service{
		workflowRepo:    workflowRepo,
		nodeRepo:        nodeRepo,
		nodeDefaultRepo: nodeDefaultRepo,
		executionRepo:   executionRepo,
		nodeResultRepo:  nodeResultRepo,
		engine:          eng,
	}
}

// ─── Workflow CRUD ──────────────────────────────────────

// CreateWorkflow creates a new workflow.
func (s *Service) CreateWorkflow(ctx context.Context, ownerType domain.OwnerType, ownerID uint64, req *CreateWorkflowRequest) (*WorkflowResponse, error) {
	profile, initialNodes, err := buildInitialWorkflow(req.WorkflowType)
	if err != nil {
		return nil, err
	}
	wf := &domain.Workflow{
		Name:         req.Name,
		Description:  req.Description,
		WorkflowType: profile.WorkflowType,
		SourceKind:   profile.SourceKind,
		TargetKind:   profile.TargetKind,
		IsLegacy:     profile.IsLegacy,
		OwnerType:    ownerType,
		OwnerID:      ownerID,
		SourceID:     req.SourceID,
	}
	if err := s.workflowRepo.Create(ctx, wf); err != nil {
		return nil, err
	}
	if len(initialNodes) == 0 {
		return ToWorkflowResponse(wf), nil
	}
	if len(initialNodes) > 0 {
		if s.nodeRepo == nil {
			return nil, fmt.Errorf("node repository is not configured")
		}
		for i := range initialNodes {
			initialNodes[i].WorkflowID = wf.ID
			initialNodes[i].Position = i + 1
		}
		if err := s.nodeRepo.BatchSave(ctx, wf.ID, initialNodes); err != nil {
			return nil, err
		}
	}
	wf.Nodes = initialNodes
	return ToWorkflowResponse(wf), nil
}

// GetWorkflow returns a workflow with its nodes.
func (s *Service) GetWorkflow(ctx context.Context, id uint64) (*WorkflowResponse, error) {
	wf, err := s.workflowRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	nodes, err := s.nodeRepo.ListByWorkflow(ctx, id)
	if err != nil {
		return nil, err
	}
	wf.Nodes = nodes
	s.ensureWorkflowProfile(wf, nodes)
	return ToWorkflowResponse(wf), nil
}

// UpdateWorkflow updates workflow metadata.
func (s *Service) UpdateWorkflow(ctx context.Context, id uint64, req *UpdateWorkflowRequest) (*WorkflowResponse, error) {
	wf, err := s.workflowRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	wf.Name = req.Name
	wf.Description = req.Description
	if req.IsPublished != nil {
		wf.IsPublished = *req.IsPublished
	}
	nodes, err := s.nodeRepo.ListByWorkflow(ctx, id)
	if err != nil {
		return nil, err
	}
	s.ensureWorkflowProfile(wf, nodes)
	if err := s.workflowRepo.Update(ctx, wf); err != nil {
		return nil, err
	}
	wf.Nodes = nodes
	return ToWorkflowResponse(wf), nil
}

// DeleteWorkflow deletes a workflow and its nodes.
func (s *Service) DeleteWorkflow(ctx context.Context, id uint64) error {
	if err := s.nodeRepo.DeleteByWorkflow(ctx, id); err != nil {
		return err
	}
	return s.workflowRepo.Delete(ctx, id)
}

// ListWorkflows lists workflows with optional filters.
func (s *Service) ListWorkflows(ctx context.Context, ownerType *domain.OwnerType, ownerID *uint64, publishedOnly bool, offset, limit int) (*WorkflowListResponse, error) {
	items, total, err := s.workflowRepo.List(ctx, ownerType, ownerID, publishedOnly, offset, limit)
	if err != nil {
		return nil, err
	}
	return s.toWorkflowListResponse(ctx, items, total)
}

func (s *Service) ListWorkflowsFiltered(ctx context.Context, ownerType *domain.OwnerType, ownerID *uint64, publishedOnly bool, offset, limit int, filter WorkflowListFilter) (*WorkflowListResponse, error) {
	items, total, err := s.workflowRepo.ListFiltered(ctx, ownerType, ownerID, publishedOnly, filter.toDomainFilter(), offset, limit)
	if err != nil {
		return nil, err
	}
	return s.toWorkflowListResponse(ctx, items, total)
}

// CloneWorkflow clones an existing workflow for a user.
func (s *Service) CloneWorkflow(ctx context.Context, sourceID, userID uint64) (*WorkflowResponse, error) {
	source, err := s.workflowRepo.GetByID(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("source workflow not found: %w", err)
	}
	nodes, err := s.nodeRepo.ListByWorkflow(ctx, sourceID)
	if err != nil {
		return nil, err
	}

	clone := &domain.Workflow{
		Name:              source.Name + " (副本)",
		Description:       source.Description,
		WorkflowType:      source.WorkflowType,
		SourceKind:        source.SourceKind,
		TargetKind:        source.TargetKind,
		IsLegacy:          source.IsLegacy,
		ValidationMessage: source.ValidationMessage,
		OwnerType:         domain.OwnerUser,
		OwnerID:           userID,
		SourceID:          &sourceID,
	}
	if err := s.workflowRepo.Create(ctx, clone); err != nil {
		return nil, err
	}

	if len(nodes) > 0 {
		clonedNodes := make([]domain.Node, len(nodes))
		for i, n := range nodes {
			clonedNodes[i] = domain.Node{
				WorkflowID: clone.ID,
				NodeType:   n.NodeType,
				Position:   n.Position,
				Config:     n.Config,
				Enabled:    n.Enabled,
			}
		}
		if err := s.nodeRepo.BatchSave(ctx, clone.ID, clonedNodes); err != nil {
			return nil, err
		}
		if err := s.syncWorkflowProfile(ctx, clone, clonedNodes); err != nil {
			return nil, err
		}
	}

	return s.GetWorkflow(ctx, clone.ID)
}

// ─── Node Management ────────────────────────────────────

// BatchUpdateNodes replaces all nodes for a workflow.
func (s *Service) BatchUpdateNodes(ctx context.Context, workflowID uint64, req *BatchUpdateNodesRequest) (*WorkflowResponse, error) {
	// Verify workflow exists
	wf, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	currentNodes, err := s.nodeRepo.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	expectedProfile, err := profileForExistingWorkflow(wf, currentNodes)
	if err != nil {
		return nil, err
	}

	nodes := make([]domain.Node, len(req.Nodes))
	for i, nr := range req.Nodes {
		nodeType := domain.NodeType(nr.NodeType)
		if !nodeType.Valid() {
			return nil, fmt.Errorf("invalid node_type: %s", nr.NodeType)
		}
		enabled := true
		if nr.Enabled != nil {
			enabled = *nr.Enabled
		}
		config := string(nr.Config)
		if config == "" {
			config = "{}"
		}
		nodes[i] = domain.Node{
			WorkflowID: workflowID,
			NodeType:   nodeType,
			Position:   nr.Position,
			Config:     config,
			Enabled:    enabled,
		}
	}

	if err := validateFixedWorkflowBoundary(expectedProfile, nodes); err != nil {
		return nil, err
	}
	profile, err := deriveWorkflowProfile(nodes)
	if err != nil {
		return nil, err
	}
	if err := ensureWorkflowProfileLocked(expectedProfile, profile); err != nil {
		return nil, err
	}
	wf.WorkflowType = profile.WorkflowType
	wf.SourceKind = profile.SourceKind
	wf.TargetKind = profile.TargetKind
	wf.IsLegacy = profile.IsLegacy
	wf.ValidationMessage = profile.ValidationMessage

	if err := s.nodeRepo.BatchSave(ctx, workflowID, nodes); err != nil {
		return nil, err
	}
	if err := s.workflowRepo.Update(ctx, wf); err != nil {
		return nil, err
	}

	return s.GetWorkflow(ctx, workflowID)
}

// ─── Execution ──────────────────────────────────────────

// ExecuteWorkflow runs a workflow against input text and records the execution.
func (s *Service) ExecuteWorkflow(ctx context.Context, workflowID uint64, triggerType domain.TriggerType, triggerID string, inputText string, meta *engine.ExecutionMeta) (*ExecutionResponse, error) {
	nodes, err := s.nodeRepo.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	resolvedNodes, err := s.resolveWorkflowNodes(ctx, nodes)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	exec := &domain.Execution{
		WorkflowID:  workflowID,
		TriggerType: triggerType,
		TriggerID:   triggerID,
		InputText:   inputText,
		Status:      domain.ExecStatusRunning,
		StartedAt:   &now,
	}
	if err := s.executionRepo.Create(ctx, exec); err != nil {
		return nil, err
	}

	// Run the engine
	result, execErr := s.engine.Execute(ctx, resolvedNodes, inputText, meta)

	// Record results
	completedAt := time.Now()
	exec.CompletedAt = &completedAt

	if execErr != nil {
		exec.Status = domain.ExecStatusFailed
		exec.ErrorMessage = execErr.Error()
	} else {
		exec.Status = domain.ExecStatusCompleted
	}
	exec.FinalText = result.FinalText

	_ = s.executionRepo.Update(ctx, exec)

	// Save node results
	if len(result.NodeResults) > 0 {
		for i := range result.NodeResults {
			result.NodeResults[i].ExecutionID = exec.ID
		}
		_ = s.nodeResultRepo.BatchCreate(ctx, result.NodeResults)
	}

	// Reload for response
	exec.NodeResults = result.NodeResults
	return ToExecutionResponse(exec), execErr
}

// TestNode tests a single node handler with sample input.
func (s *Service) TestNode(ctx context.Context, req *TestNodeRequest) (*TestNodeResponse, error) {
	nodeType := domain.NodeType(req.NodeType)
	if !nodeType.Valid() {
		return nil, fmt.Errorf("invalid node_type: %s", req.NodeType)
	}
	if nodeType.IsSource() {
		return &TestNodeResponse{
			Error: "源节点仅用于声明输入来源，不能做文本级单节点测试。请测试后续处理节点，或执行整条工作流。",
		}, nil
	}
	resolvedConfig, err := s.resolveNodeConfig(ctx, nodeType, req.Config)
	if err != nil {
		return nil, err
	}

	result, err := s.engine.TestNode(ctx, nodeType, resolvedConfig, req.InputText, &engine.ExecutionMeta{
		AudioURL:      req.AudioURL,
		AudioFilePath: req.AudioFilePath,
	})
	if err != nil {
		return &TestNodeResponse{Error: err.Error()}, nil
	}

	return &TestNodeResponse{
		OutputText: result.OutputText,
		Detail:     result.Detail,
		DurationMs: result.DurationMs,
		Error:      result.Error,
	}, nil
}

func (s *Service) TestNodeStream(ctx context.Context, req *TestNodeRequest, emit TestNodeStreamEmitter) (*TestNodeResponse, error) {
	nodeType := domain.NodeType(req.NodeType)
	if !nodeType.Valid() {
		return nil, fmt.Errorf("invalid node_type: %s", req.NodeType)
	}
	resolvedConfig, err := s.resolveNodeConfig(ctx, nodeType, req.Config)
	if err != nil {
		return nil, err
	}

	result, err := s.engine.TestNodeStream(ctx, nodeType, resolvedConfig, req.InputText, &engine.ExecutionMeta{
		AudioURL:      req.AudioURL,
		AudioFilePath: req.AudioFilePath,
	}, func(event *engine.NodeStreamEvent) error {
		if emit == nil || event == nil {
			return nil
		}
		return emit(&TestNodeStreamEvent{
			Type:       string(event.Type),
			Message:    event.Message,
			Delta:      event.Delta,
			OutputText: event.OutputText,
			Detail:     event.Detail,
			DurationMs: event.DurationMs,
			Error:      event.Error,
		})
	})
	if err != nil {
		return nil, err
	}

	return &TestNodeResponse{
		OutputText: result.OutputText,
		Detail:     result.Detail,
		DurationMs: result.DurationMs,
		Error:      result.Error,
	}, nil
}

// GetExecution returns an execution with its node results.
func (s *Service) GetExecution(ctx context.Context, id uint64) (*ExecutionResponse, error) {
	exec, err := s.executionRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	nodeResults, err := s.nodeResultRepo.ListByExecution(ctx, id)
	if err != nil {
		return nil, err
	}
	exec.NodeResults = nodeResults
	return ToExecutionResponse(exec), nil
}

// ListExecutionsByTask returns workflow executions triggered by a transcription task.
func (s *Service) ListExecutionsByTask(ctx context.Context, taskID uint64, offset, limit int) ([]*ExecutionResponse, error) {
	triggerID := fmt.Sprintf("%d", taskID)
	batchItems, _, err := s.executionRepo.ListByTrigger(ctx, domain.TriggerBatchTask, triggerID, 0, 1000)
	if err != nil {
		return nil, err
	}
	realtimeItems, _, err := s.executionRepo.ListByTrigger(ctx, domain.TriggerRealtime, triggerID, 0, 1000)
	if err != nil {
		return nil, err
	}

	all := make([]*domain.Execution, 0, len(batchItems)+len(realtimeItems))
	all = append(all, batchItems...)
	all = append(all, realtimeItems...)

	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = len(all)
	}
	if offset > len(all) {
		return []*ExecutionResponse{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	page := all[offset:end]
	resp := make([]*ExecutionResponse, 0, len(page))
	for _, exec := range page {
		nodeResults, err := s.nodeResultRepo.ListByExecution(ctx, exec.ID)
		if err != nil {
			return nil, err
		}
		exec.NodeResults = nodeResults
		resp = append(resp, ToExecutionResponse(exec))
	}
	return resp, nil
}

// GetNodeTypes returns all registered node types.
func (s *Service) GetNodeTypes(ctx context.Context) ([]NodeTypeInfo, error) {
	types := domain.AllNodeTypes()
	infos := make([]NodeTypeInfo, len(types))
	for i, t := range types {
		defaultConfig, err := s.resolveGlobalNodeDefault(ctx, t)
		if err != nil {
			return nil, err
		}
		infos[i] = NodeTypeInfo{
			Type:          t,
			Label:         t.Label(),
			Role:          t.Role(),
			Description:   t.Description(),
			DefaultConfig: defaultConfig,
		}
	}
	return infos, nil
}

// GetNodeDefaultConfig returns the merged global default configuration for a node type.
// Used by callers (e.g. voice control intent classifier) that need to reuse the
// admin-configured LLM endpoint without going through the full workflow pipeline.
func (s *Service) GetNodeDefaultConfig(ctx context.Context, nodeType domain.NodeType) (json.RawMessage, error) {
	if !nodeType.Valid() {
		return nil, fmt.Errorf("invalid node_type: %s", nodeType)
	}
	return s.resolveGlobalNodeDefault(ctx, nodeType)
}

// UpdateNodeDefault updates the persisted default configuration for a node type.
func (s *Service) UpdateNodeDefault(ctx context.Context, nodeType domain.NodeType, req *UpdateNodeDefaultRequest) (*NodeTypeInfo, error) {
	if !nodeType.Valid() {
		return nil, fmt.Errorf("invalid node_type: %s", nodeType)
	}
	if s.nodeDefaultRepo == nil {
		return nil, fmt.Errorf("node default repository is not configured")
	}

	resolvedConfig, err := s.resolveNodeConfig(ctx, nodeType, req.Config)
	if err != nil {
		return nil, err
	}
	if nodeType.IsSource() {
		resolvedConfig = mustMarshalNodeConfig(builtinNodeDefaultConfig(nodeType))
	}
	if !nodeType.IsSource() && s.engine != nil {
		if err := s.engine.ValidateNodeConfig(nodeType, resolvedConfig); err != nil {
			return nil, err
		}
	}

	item := &domain.NodeDefault{
		NodeType: nodeType,
		Config:   string(req.Config),
	}
	if strings.TrimSpace(item.Config) == "" {
		item.Config = "{}"
	}
	if err := s.nodeDefaultRepo.Upsert(ctx, item); err != nil {
		return nil, err
	}

	defaultConfig, err := s.resolveGlobalNodeDefault(ctx, nodeType)
	if err != nil {
		return nil, err
	}
	return &NodeTypeInfo{
		Type:          nodeType,
		Label:         nodeType.Label(),
		Role:          nodeType.Role(),
		Description:   nodeType.Description(),
		DefaultConfig: defaultConfig,
	}, nil
}

func (s *Service) resolveWorkflowNodes(ctx context.Context, nodes []domain.Node) ([]domain.Node, error) {
	resolved := make([]domain.Node, len(nodes))
	for i := range nodes {
		resolved[i] = nodes[i]
		config, err := s.resolveNodeConfig(ctx, nodes[i].NodeType, json.RawMessage(nodes[i].Config))
		if err != nil {
			return nil, fmt.Errorf("resolve config for node %s (#%d): %w", nodes[i].NodeType, nodes[i].Position, err)
		}
		resolved[i].Config = string(config)
	}
	return resolved, nil
}

// ExecuteWorkflowForTask is a convenience method for batch task post-processing.
func (s *Service) ExecuteWorkflowForTask(ctx context.Context, workflowID uint64, taskID uint64, userID uint64, inputText, audioURL, audioFilePath string) (*ExecutionResponse, error) {
	meta := &engine.ExecutionMeta{
		AudioURL:      audioURL,
		AudioFilePath: audioFilePath,
		TaskID:        taskID,
		UserID:        userID,
	}
	return s.ExecuteWorkflow(ctx, workflowID, domain.TriggerBatchTask, fmt.Sprintf("%d", taskID), inputText, meta)
}

// ExecuteWorkflowForRealtimeTask executes a workflow for a realtime transcription task.
func (s *Service) ExecuteWorkflowForRealtimeTask(ctx context.Context, workflowID uint64, taskID uint64, userID uint64, inputText, audioURL, audioFilePath string) (*ExecutionResponse, error) {
	meta := &engine.ExecutionMeta{
		AudioURL:      audioURL,
		AudioFilePath: audioFilePath,
		TaskID:        taskID,
		UserID:        userID,
	}
	return s.ExecuteWorkflow(ctx, workflowID, domain.TriggerRealtime, fmt.Sprintf("%d", taskID), inputText, meta)
}

// ResumeLatestFailedExecutionForTask continues the latest failed batch execution from its failed node.
func (s *Service) ResumeLatestFailedExecutionForTask(ctx context.Context, workflowID uint64, taskID uint64, userID uint64, audioURL, audioFilePath string) (*ExecutionResponse, error) {
	meta := &engine.ExecutionMeta{
		AudioURL:      audioURL,
		AudioFilePath: audioFilePath,
		TaskID:        taskID,
		UserID:        userID,
	}
	return s.resumeLatestFailedExecution(ctx, workflowID, domain.TriggerBatchTask, fmt.Sprintf("%d", taskID), meta)
}

// ResumeLatestFailedExecutionForRealtimeTask continues the latest failed realtime execution from its failed node.
func (s *Service) ResumeLatestFailedExecutionForRealtimeTask(ctx context.Context, workflowID uint64, taskID uint64, userID uint64, audioURL, audioFilePath string) (*ExecutionResponse, error) {
	meta := &engine.ExecutionMeta{
		AudioURL:      audioURL,
		AudioFilePath: audioFilePath,
		TaskID:        taskID,
		UserID:        userID,
	}
	return s.resumeLatestFailedExecution(ctx, workflowID, domain.TriggerRealtime, fmt.Sprintf("%d", taskID), meta)
}

func (s *Service) resumeLatestFailedExecution(ctx context.Context, workflowID uint64, triggerType domain.TriggerType, triggerID string, meta *engine.ExecutionMeta) (*ExecutionResponse, error) {
	nodes, err := s.nodeRepo.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	failedExec, previousResults, err := s.latestFailedExecutionByTrigger(ctx, workflowID, triggerType, triggerID)
	if err != nil {
		return nil, err
	}

	resumeNodes, carryResults, startInput, err := buildExecutionResumePlan(nodes, failedExec, previousResults)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	exec := &domain.Execution{
		WorkflowID:  workflowID,
		TriggerType: triggerType,
		TriggerID:   triggerID,
		InputText:   failedExec.InputText,
		Status:      domain.ExecStatusRunning,
		StartedAt:   &now,
	}
	if err := s.executionRepo.Create(ctx, exec); err != nil {
		return nil, err
	}

	result, execErr := s.engine.Execute(ctx, resumeNodes, startInput, meta)
	if result == nil {
		result = &engine.ExecuteResult{FinalText: startInput}
	}

	combinedResults := make([]domain.NodeResult, 0, len(carryResults)+len(result.NodeResults))
	combinedResults = append(combinedResults, carryResults...)
	combinedResults = append(combinedResults, result.NodeResults...)

	completedAt := time.Now()
	exec.CompletedAt = &completedAt
	if execErr != nil {
		exec.Status = domain.ExecStatusFailed
		exec.ErrorMessage = execErr.Error()
	} else {
		exec.Status = domain.ExecStatusCompleted
	}
	exec.FinalText = result.FinalText
	if err := s.executionRepo.Update(ctx, exec); err != nil {
		return nil, err
	}

	if len(combinedResults) > 0 {
		for index := range combinedResults {
			combinedResults[index].ID = 0
			combinedResults[index].ExecutionID = exec.ID
		}
		if err := s.nodeResultRepo.BatchCreate(ctx, combinedResults); err != nil {
			return nil, err
		}
	}

	exec.NodeResults = combinedResults
	return ToExecutionResponse(exec), execErr
}

func (s *Service) latestFailedExecutionByTrigger(ctx context.Context, workflowID uint64, triggerType domain.TriggerType, triggerID string) (*domain.Execution, []domain.NodeResult, error) {
	items, _, err := s.executionRepo.ListByTrigger(ctx, triggerType, triggerID, 0, 1000)
	if err != nil {
		return nil, nil, err
	}
	for _, item := range items {
		if item.WorkflowID != workflowID || item.Status != domain.ExecStatusFailed {
			continue
		}
		nodeResults, err := s.nodeResultRepo.ListByExecution(ctx, item.ID)
		if err != nil {
			return nil, nil, err
		}
		return item, nodeResults, nil
	}
	return nil, nil, fmt.Errorf("no failed workflow execution found for trigger %s:%s", triggerType, triggerID)
}

func buildExecutionResumePlan(nodes []domain.Node, failedExec *domain.Execution, previousResults []domain.NodeResult) ([]domain.Node, []domain.NodeResult, string, error) {
	if failedExec == nil {
		return nil, nil, "", fmt.Errorf("failed execution is required")
	}

	sortedNodes := make([]domain.Node, len(nodes))
	copy(sortedNodes, nodes)
	sort.Slice(sortedNodes, func(i, j int) bool {
		return sortedNodes[i].Position < sortedNodes[j].Position
	})

	carryResults := make([]domain.NodeResult, 0, len(previousResults))
	var failedNode *domain.NodeResult
	for index := range previousResults {
		result := previousResults[index]
		if result.Status == domain.NodeResultFailed {
			failedNode = &result
			break
		}
		carryResults = append(carryResults, cloneNodeResultForResume(result))
	}
	if failedNode == nil {
		return nil, nil, "", fmt.Errorf("failed execution does not contain a failed node")
	}

	startIndex := -1
	for index, node := range sortedNodes {
		if node.Position == failedNode.Position && node.NodeType == failedNode.NodeType {
			startIndex = index
			break
		}
	}
	if startIndex == -1 {
		for index, node := range sortedNodes {
			if node.Position == failedNode.Position {
				startIndex = index
				break
			}
		}
	}
	if startIndex == -1 {
		return nil, nil, "", fmt.Errorf("failed node position %d no longer exists in current workflow", failedNode.Position)
	}

	startInput := failedNode.InputText
	if startInput == "" {
		if len(carryResults) > 0 {
			startInput = carryResults[len(carryResults)-1].OutputText
		} else {
			startInput = failedExec.InputText
		}
	}

	return sortedNodes[startIndex:], carryResults, startInput, nil
}

func cloneNodeResultForResume(result domain.NodeResult) domain.NodeResult {
	return domain.NodeResult{
		NodeID:     result.NodeID,
		NodeType:   result.NodeType,
		Position:   result.Position,
		InputText:  result.InputText,
		OutputText: result.OutputText,
		Status:     result.Status,
		Detail:     result.Detail,
		DurationMs: result.DurationMs,
		ExecutedAt: result.ExecutedAt,
	}
}

// ValidateWorkflowBinding ensures a workflow can be bound to the given application entry type.
func (s *Service) ValidateWorkflowBinding(ctx context.Context, workflowID uint64, expectedType domain.WorkflowType) (*domain.Workflow, error) {
	wf, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow not found")
	}

	nodes, err := s.nodeRepo.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}

	profile, err := deriveWorkflowProfile(nodes)
	if err != nil {
		return nil, err
	}
	wf.Nodes = nodes
	wf.WorkflowType = profile.WorkflowType
	wf.SourceKind = profile.SourceKind
	wf.TargetKind = profile.TargetKind
	wf.IsLegacy = profile.IsLegacy
	wf.ValidationMessage = profile.ValidationMessage

	if wf.IsLegacy {
		return nil, fmt.Errorf("workflow %d 仍是 legacy 工作流，不能绑定到%s入口", workflowID, expectedType.Label())
	}
	if wf.WorkflowType != expectedType {
		return nil, fmt.Errorf("workflow %d 的类型是%s，不能绑定到%s入口", workflowID, wf.WorkflowType.Label(), expectedType.Label())
	}
	return wf, nil
}

// GetWorkflowByID is a thin wrapper used by other services.
func (s *Service) GetWorkflowByID(ctx context.Context, id uint64) (*domain.Workflow, error) {
	return s.workflowRepo.GetByID(ctx, id)
}

// ListPublishedWorkflows returns all published workflows for a given scope.
func (s *Service) ListPublishedWorkflows(ctx context.Context, offset, limit int) (*WorkflowListResponse, error) {
	return s.ListWorkflows(ctx, nil, nil, true, offset, limit)
}

// ListUserWorkflows returns workflows owned by a specific user, plus published system workflows.
func (s *Service) ListUserAccessibleWorkflows(ctx context.Context, userID uint64, offset, limit int) (*WorkflowListResponse, error) {
	// Get user's own workflows
	userType := domain.OwnerUser
	userItems, _, err := s.workflowRepo.List(ctx, &userType, &userID, false, 0, 0)
	if err != nil {
		return nil, err
	}

	// Get published system workflows
	sysType := domain.OwnerSystem
	sysItems, _, err := s.workflowRepo.List(ctx, &sysType, nil, true, 0, 0)
	if err != nil {
		return nil, err
	}

	all := make([]*domain.Workflow, 0, len(userItems)+len(sysItems))
	all = append(all, sysItems...)
	all = append(all, userItems...)
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	total := int64(len(all))
	if total == 0 {
		return &WorkflowListResponse{Items: []*WorkflowResponse{}, Total: 0}, nil
	}

	list, err := s.toWorkflowListResponse(ctx, all, total)
	if err != nil {
		return nil, err
	}
	return paginateWorkflowList(list, offset, limit), nil
}

func (s *Service) ListUserAccessibleWorkflowsFiltered(ctx context.Context, userID uint64, offset, limit int, filter WorkflowListFilter) (*WorkflowListResponse, error) {
	domainFilter := filter.toDomainFilter()
	userType := domain.OwnerUser
	userItems, _, err := s.workflowRepo.ListFiltered(ctx, &userType, &userID, false, domainFilter, 0, 0)
	if err != nil {
		return nil, err
	}
	sysType := domain.OwnerSystem
	sysItems, _, err := s.workflowRepo.ListFiltered(ctx, &sysType, nil, true, domainFilter, 0, 0)
	if err != nil {
		return nil, err
	}
	all := make([]*domain.Workflow, 0, len(userItems)+len(sysItems))
	all = append(all, sysItems...)
	all = append(all, userItems...)
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})
	list, err := s.toWorkflowListResponse(ctx, all, int64(len(all)))
	if err != nil {
		return nil, err
	}
	return paginateWorkflowList(list, offset, limit), nil
}

type WorkflowListFilter struct {
	WorkflowType  *domain.WorkflowType
	SourceKind    *domain.WorkflowSourceKind
	TargetKind    *domain.WorkflowTargetKind
	IncludeLegacy bool
}

func (f WorkflowListFilter) toDomainFilter() domain.WorkflowListFilter {
	return domain.WorkflowListFilter{
		WorkflowType:  f.WorkflowType,
		SourceKind:    f.SourceKind,
		TargetKind:    f.TargetKind,
		IncludeLegacy: f.IncludeLegacy,
	}
}

func (s *Service) FilterWorkflowList(list *WorkflowListResponse, filter WorkflowListFilter) *WorkflowListResponse {
	if list == nil {
		return &WorkflowListResponse{Items: []*WorkflowResponse{}}
	}
	if filter.IncludeLegacy && filter.WorkflowType == nil && filter.SourceKind == nil && filter.TargetKind == nil {
		return list
	}
	items := make([]*WorkflowResponse, 0, len(list.Items))
	for _, item := range list.Items {
		if item == nil {
			continue
		}
		if !filter.IncludeLegacy && item.IsLegacy {
			continue
		}
		if filter.WorkflowType != nil && item.WorkflowType != *filter.WorkflowType {
			continue
		}
		if filter.SourceKind != nil && item.SourceKind != *filter.SourceKind {
			continue
		}
		if filter.TargetKind != nil && item.TargetKind != *filter.TargetKind {
			continue
		}
		items = append(items, item)
	}
	return &WorkflowListResponse{Items: items, Total: int64(len(items))}
}

func (f WorkflowListFilter) RequiresPrePagination() bool {
	return !f.IncludeLegacy || f.WorkflowType != nil || f.SourceKind != nil || f.TargetKind != nil
}

func (s *Service) toWorkflowListResponse(ctx context.Context, items []*domain.Workflow, total int64) (*WorkflowListResponse, error) {
	resp := &WorkflowListResponse{Total: total}
	for _, wf := range items {
		nodes, err := s.nodeRepo.ListByWorkflow(ctx, wf.ID)
		if err != nil {
			return nil, err
		}
		wf.Nodes = nodes
		s.ensureWorkflowProfile(wf, nodes)
		resp.Items = append(resp.Items, ToWorkflowResponse(wf))
	}
	return resp, nil
}

func paginateWorkflowList(list *WorkflowListResponse, offset, limit int) *WorkflowListResponse {
	if list == nil {
		return &WorkflowListResponse{Items: []*WorkflowResponse{}, Total: 0}
	}
	total := int64(len(list.Items))
	if total == 0 {
		return &WorkflowListResponse{Items: []*WorkflowResponse{}, Total: 0}
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = len(list.Items)
	}
	if offset >= len(list.Items) {
		return &WorkflowListResponse{Items: []*WorkflowResponse{}, Total: total}
	}
	end := offset + limit
	if end > len(list.Items) {
		end = len(list.Items)
	}
	return &WorkflowListResponse{
		Items: list.Items[offset:end],
		Total: total,
	}
}

type workflowProfile struct {
	WorkflowType      domain.WorkflowType
	SourceKind        domain.WorkflowSourceKind
	TargetKind        domain.WorkflowTargetKind
	IsLegacy          bool
	ValidationMessage string
}

func (s *Service) syncWorkflowProfile(ctx context.Context, wf *domain.Workflow, nodes []domain.Node) error {
	if wf == nil {
		return nil
	}
	profile, err := deriveWorkflowProfile(nodes)
	if err != nil {
		return err
	}
	wf.WorkflowType = profile.WorkflowType
	wf.SourceKind = profile.SourceKind
	wf.TargetKind = profile.TargetKind
	wf.IsLegacy = profile.IsLegacy
	wf.ValidationMessage = profile.ValidationMessage
	return s.workflowRepo.Update(ctx, wf)
}

func (s *Service) ensureWorkflowProfile(wf *domain.Workflow, nodes []domain.Node) {
	if wf == nil {
		return
	}
	if wf.WorkflowType != "" && wf.SourceKind != "" && wf.TargetKind != "" {
		return
	}
	profile, err := deriveWorkflowProfile(nodes)
	if err != nil {
		wf.WorkflowType = domain.WorkflowTypeLegacy
		wf.SourceKind = domain.SourceKindLegacyText
		wf.TargetKind = domain.TargetKindTranscript
		wf.IsLegacy = true
		wf.ValidationMessage = err.Error()
		return
	}
	wf.WorkflowType = profile.WorkflowType
	wf.SourceKind = profile.SourceKind
	wf.TargetKind = profile.TargetKind
	wf.IsLegacy = profile.IsLegacy
	wf.ValidationMessage = profile.ValidationMessage
}

func deriveWorkflowProfile(nodes []domain.Node) (*workflowProfile, error) {
	enabled := enabledNodesForProfile(nodes)
	if len(enabled) == 0 {
		return &workflowProfile{
			WorkflowType:      domain.WorkflowTypeLegacy,
			SourceKind:        domain.SourceKindLegacyText,
			TargetKind:        domain.TargetKindTranscript,
			IsLegacy:          true,
			ValidationMessage: "当前工作流还没有启用节点，暂按旧版文本后处理兼容。",
		}, nil
	}

	sourceCount := 0
	meetingSummaryCount := 0
	for _, node := range enabled {
		if node.NodeType.IsSource() {
			sourceCount++
		}
		if node.NodeType == domain.NodeMeetingSummary {
			meetingSummaryCount++
		}
	}

	if meetingSummaryCount > 1 {
		return nil, fmt.Errorf("meeting_summary 节点最多只能启用一个")
	}
	if meetingSummaryCount == 1 && enabled[len(enabled)-1].NodeType != domain.NodeMeetingSummary {
		return nil, fmt.Errorf("meeting_summary 节点必须位于最后一位")
	}

	if sourceCount == 0 {
		if deriveTargetKind(enabled) == domain.TargetKindVoiceCommand {
			return &workflowProfile{
				WorkflowType:      domain.WorkflowTypeVoice,
				SourceKind:        domain.SourceKindLegacyText,
				TargetKind:        domain.TargetKindVoiceCommand,
				ValidationMessage: "缺少唤醒源节点，当前按兼容模式保留为语音控制工作流；建议补齐 voice_wake 节点。",
			}, nil
		}
		return &workflowProfile{
			WorkflowType:      domain.WorkflowTypeLegacy,
			SourceKind:        domain.SourceKindLegacyText,
			TargetKind:        deriveTargetKind(enabled),
			IsLegacy:          true,
			ValidationMessage: "缺少 ASR 源节点，当前仍按旧版文本后处理工作流兼容。",
		}, nil
	}
	if sourceCount > 1 {
		return nil, fmt.Errorf("严格模式下必须且只能启用一个源节点")
	}
	if !enabled[0].NodeType.IsSource() {
		return nil, fmt.Errorf("源节点必须位于第一位")
	}

	sourceKind := domain.SourceKindBatchASR
	switch enabled[0].NodeType {
	case domain.NodeBatchASR:
		sourceKind = domain.SourceKindBatchASR
	case domain.NodeRealtimeASR:
		sourceKind = domain.SourceKindRealtimeASR
	case domain.NodeVoiceWake:
		sourceKind = domain.SourceKindVoiceWake
	default:
		return nil, fmt.Errorf("不支持的源节点类型: %s", enabled[0].NodeType)
	}

	profile := &workflowProfile{
		SourceKind: sourceKind,
		TargetKind: deriveTargetKind(enabled),
	}
	if profile.TargetKind == domain.TargetKindMeetingSummary {
		profile.WorkflowType = domain.WorkflowTypeMeeting
	} else if profile.TargetKind == domain.TargetKindVoiceCommand {
		profile.WorkflowType = domain.WorkflowTypeVoice
	} else if sourceKind == domain.SourceKindRealtimeASR {
		profile.WorkflowType = domain.WorkflowTypeRealtime
	} else {
		profile.WorkflowType = domain.WorkflowTypeBatch
	}
	return profile, nil
}

func enabledNodesForProfile(nodes []domain.Node) []domain.Node {
	items := make([]domain.Node, 0, len(nodes))
	for _, node := range nodes {
		if !node.Enabled {
			continue
		}
		items = append(items, node)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Position == items[j].Position {
			return string(items[i].NodeType) < string(items[j].NodeType)
		}
		return items[i].Position < items[j].Position
	})
	return items
}

func deriveTargetKind(nodes []domain.Node) domain.WorkflowTargetKind {
	for _, node := range nodes {
		if node.NodeType == domain.NodeMeetingSummary {
			return domain.TargetKindMeetingSummary
		}
		if node.NodeType == domain.NodeVoiceIntent {
			return domain.TargetKindVoiceCommand
		}
	}
	return domain.TargetKindTranscript
}

func buildInitialWorkflow(workflowType *domain.WorkflowType) (*workflowProfile, []domain.Node, error) {
	if workflowType == nil || *workflowType == "" || *workflowType == domain.WorkflowTypeLegacy {
		return &workflowProfile{
			WorkflowType: domain.WorkflowTypeLegacy,
			SourceKind:   domain.SourceKindLegacyText,
			TargetKind:   domain.TargetKindTranscript,
			IsLegacy:     true,
		}, nil, nil
	}

	profile := &workflowProfile{WorkflowType: *workflowType}
	switch *workflowType {
	case domain.WorkflowTypeBatch:
		profile.SourceKind = domain.SourceKindBatchASR
		profile.TargetKind = domain.TargetKindTranscript
	case domain.WorkflowTypeRealtime:
		profile.SourceKind = domain.SourceKindRealtimeASR
		profile.TargetKind = domain.TargetKindTranscript
	case domain.WorkflowTypeMeeting:
		profile.SourceKind = domain.SourceKindBatchASR
		profile.TargetKind = domain.TargetKindMeetingSummary
	case domain.WorkflowTypeVoice:
		profile.SourceKind = domain.SourceKindVoiceWake
		profile.TargetKind = domain.TargetKindVoiceCommand
	default:
		return nil, nil, fmt.Errorf("invalid workflow_type: %s", *workflowType)
	}

	nodes := make([]domain.Node, 0, 2)
	if sourceType, ok := profile.SourceKind.NodeType(); ok {
		nodes = append(nodes, domain.Node{
			NodeType: sourceType,
			Enabled:  true,
			Config:   "{}",
		})
	}
	if sinkType, ok := profile.TargetKind.FixedSinkNodeType(); ok {
		nodes = append(nodes, domain.Node{
			NodeType: sinkType,
			Enabled:  true,
			Config:   "{}",
		})
	}
	return profile, nodes, nil
}

func profileForExistingWorkflow(wf *domain.Workflow, nodes []domain.Node) (*workflowProfile, error) {
	if len(nodes) > 0 {
		return deriveWorkflowProfile(nodes)
	}
	return &workflowProfile{
		WorkflowType: wf.WorkflowType,
		SourceKind:   wf.SourceKind,
		TargetKind:   wf.TargetKind,
		IsLegacy:     wf.IsLegacy,
	}, nil
}

func ensureWorkflowProfileLocked(expected, actual *workflowProfile) error {
	if expected == nil || actual == nil {
		return nil
	}
	if expected.WorkflowType != actual.WorkflowType || expected.SourceKind != actual.SourceKind || expected.TargetKind != actual.TargetKind || expected.IsLegacy != actual.IsLegacy {
		return fmt.Errorf("工作流类型在创建时已确定，当前仅允许编辑节点配置和中间处理链路，不能改变入口/出口场景")
	}
	return nil
}

func validateFixedWorkflowBoundary(profile *workflowProfile, nodes []domain.Node) error {
	if profile == nil || len(nodes) == 0 {
		return nil
	}

	ordered := append([]domain.Node(nil), nodes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Position == ordered[j].Position {
			return string(ordered[i].NodeType) < string(ordered[j].NodeType)
		}
		return ordered[i].Position < ordered[j].Position
	})

	if sourceType, ok := profile.SourceKind.NodeType(); ok {
		sourceCount := 0
		for _, node := range ordered {
			if node.NodeType.IsSource() {
				sourceCount++
			}
		}
		if sourceCount != 1 {
			return fmt.Errorf("固化源节点必须保留且只能保留一个")
		}
		if ordered[0].NodeType != sourceType {
			return fmt.Errorf("固化源节点必须保持在第一位")
		}
		if !ordered[0].Enabled {
			return fmt.Errorf("固化源节点不能被禁用")
		}
	}

	if sinkType, ok := profile.TargetKind.FixedSinkNodeType(); ok {
		sinkCount := 0
		for _, node := range ordered {
			if node.NodeType == sinkType {
				sinkCount++
			}
		}
		if sinkCount != 1 {
			return fmt.Errorf("固化输出节点必须保留且只能保留一个")
		}
		last := ordered[len(ordered)-1]
		if last.NodeType != sinkType {
			return fmt.Errorf("固化输出节点必须保持在最后一位")
		}
		if !last.Enabled {
			return fmt.Errorf("固化输出节点不能被禁用")
		}
	}

	return nil
}
