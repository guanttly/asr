package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	domain "github.com/lgt/asr/internal/domain/workflow"
	"go.uber.org/zap"
)

// Engine orchestrates the sequential execution of workflow nodes.
type Engine struct {
	handlers map[domain.NodeType]NodeHandler
	logger   *zap.Logger
}

// NewEngine creates a new workflow engine.
func NewEngine(logger *zap.Logger) *Engine {
	return &Engine{
		handlers: make(map[domain.NodeType]NodeHandler),
		logger:   logger,
	}
}

// RegisterHandler registers a node handler for the given node type.
func (e *Engine) RegisterHandler(nodeType domain.NodeType, handler NodeHandler) {
	e.handlers[nodeType] = handler
}

// NodeTestResult is the result of testing a single node.
type NodeTestResult struct {
	OutputText string          `json:"output_text"`
	Detail     json.RawMessage `json:"detail,omitempty"`
	DurationMs int             `json:"duration_ms"`
	Error      string          `json:"error,omitempty"`
}

// TestNode executes a single node handler against sample input.
func (e *Engine) TestNode(ctx context.Context, nodeType domain.NodeType, config json.RawMessage, inputText string, meta *ExecutionMeta) (*NodeTestResult, error) {
	if nodeType.IsSource() {
		return &NodeTestResult{
			DurationMs: 0,
			Error:      "源节点仅用于声明输入来源，不能做文本级单节点测试。请测试后续处理节点，或执行整条工作流。",
		}, nil
	}

	handler, ok := e.handlers[nodeType]
	if !ok {
		return nil, fmt.Errorf("unsupported node type: %s", nodeType)
	}

	if err := handler.Validate(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	start := time.Now()
	outputText, detail, err := handler.Execute(ctx, config, inputText, meta)
	durationMs := int(time.Since(start).Milliseconds())

	result := &NodeTestResult{
		OutputText: outputText,
		Detail:     detail,
		DurationMs: durationMs,
	}
	if err != nil {
		result.Error = err.Error()
	}
	return result, nil
}

// ExecuteResult is the result of executing an entire workflow.
type ExecuteResult struct {
	FinalText   string              `json:"final_text"`
	NodeResults []domain.NodeResult `json:"node_results"`
	Error       string              `json:"error,omitempty"`
}

// Execute runs all enabled nodes in order against the input text.
func (e *Engine) Execute(ctx context.Context, nodes []domain.Node, inputText string, meta *ExecutionMeta) (*ExecuteResult, error) {
	// Sort nodes by position
	sorted := make([]domain.Node, len(nodes))
	copy(sorted, nodes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Position < sorted[j].Position
	})

	currentText := inputText
	var nodeResults []domain.NodeResult

	for _, node := range sorted {
		nr := domain.NodeResult{
			NodeID:    node.ID,
			NodeType:  node.NodeType,
			Position:  node.Position,
			InputText: currentText,
		}

		if !node.Enabled {
			nr.Status = domain.NodeResultSkipped
			nr.OutputText = currentText
			now := time.Now()
			nr.ExecutedAt = &now
			nodeResults = append(nodeResults, nr)
			continue
		}

		if node.NodeType.IsSource() {
			detail, _ := json.Marshal(map[string]string{
				"mode":        "declarative_source",
				"source_kind": string(node.NodeType),
			})
			nr.Status = domain.NodeResultSuccess
			nr.OutputText = currentText
			nr.Detail = string(detail)
			now := time.Now()
			nr.ExecutedAt = &now
			nodeResults = append(nodeResults, nr)
			continue
		}

		handler, ok := e.handlers[node.NodeType]
		if !ok {
			nr.Status = domain.NodeResultFailed
			nr.OutputText = currentText
			nr.Detail = fmt.Sprintf(`{"error":"unsupported node type: %s"}`, node.NodeType)
			now := time.Now()
			nr.ExecutedAt = &now
			nodeResults = append(nodeResults, nr)
			return &ExecuteResult{
				FinalText:   currentText,
				NodeResults: nodeResults,
				Error:       fmt.Sprintf("unsupported node type: %s", node.NodeType),
			}, fmt.Errorf("unsupported node type: %s", node.NodeType)
		}

		start := time.Now()
		outputText, detail, err := handler.Execute(ctx, json.RawMessage(node.Config), currentText, meta)
		durationMs := int(time.Since(start).Milliseconds())
		now := time.Now()

		nr.DurationMs = durationMs
		nr.ExecutedAt = &now

		if err != nil {
			e.logger.Warn("workflow node execution failed",
				zap.String("node_type", string(node.NodeType)),
				zap.Uint64("node_id", node.ID),
				zap.Error(err),
			)
			nr.Status = domain.NodeResultFailed
			nr.OutputText = currentText
			errDetail, _ := json.Marshal(map[string]string{"error": err.Error()})
			nr.Detail = string(errDetail)
			nodeResults = append(nodeResults, nr)
			return &ExecuteResult{
				FinalText:   currentText,
				NodeResults: nodeResults,
				Error:       fmt.Sprintf("node %s (#%d) failed: %v", node.NodeType, node.Position, err),
			}, err
		}

		nr.Status = domain.NodeResultSuccess
		nr.OutputText = outputText
		if detail != nil {
			nr.Detail = string(detail)
		}
		nodeResults = append(nodeResults, nr)
		currentText = outputText
	}

	return &ExecuteResult{
		FinalText:   currentText,
		NodeResults: nodeResults,
	}, nil
}

// GetRegisteredTypes returns all registered node types.
func (e *Engine) GetRegisteredTypes() []domain.NodeType {
	types := make([]domain.NodeType, 0, len(e.handlers))
	for t := range e.handlers {
		types = append(types, t)
	}
	return types
}
