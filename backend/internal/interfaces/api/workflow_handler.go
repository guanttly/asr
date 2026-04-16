package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	appasr "github.com/lgt/asr/internal/application/asr"
	appwf "github.com/lgt/asr/internal/application/workflow"
	domain "github.com/lgt/asr/internal/domain/workflow"
	wfengine "github.com/lgt/asr/internal/infrastructure/workflow"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// WorkflowHandler handles HTTP requests for workflow management.
type WorkflowHandler struct {
	service    *appwf.Service
	asrService *appasr.Service
}

// NewWorkflowHandler creates a new workflow handler.
func NewWorkflowHandler(service *appwf.Service, asrService *appasr.Service) *WorkflowHandler {
	return &WorkflowHandler{service: service, asrService: asrService}
}

// Register registers workflow routes on the given router group.
func (h *WorkflowHandler) Register(group *gin.RouterGroup) {
	wf := group.Group("/workflows")
	{
		wf.GET("", h.ListWorkflows)
		wf.POST("", h.CreateWorkflow)
		wf.GET("/node-types", h.GetNodeTypes)
		wf.PUT("/node-defaults/:nodeType", h.UpdateNodeDefault)
		wf.POST("/test-node", h.TestNode)
		wf.GET("/:id", h.GetWorkflow)
		wf.PUT("/:id", h.UpdateWorkflow)
		wf.DELETE("/:id", h.DeleteWorkflow)
		wf.PUT("/:id/nodes", h.BatchUpdateNodes)
		wf.POST("/:id/execute", h.ExecuteWorkflow)
		wf.POST("/:id/clone", h.CloneWorkflow)
	}

	group.GET("/workflow-executions/:id", h.GetExecution)
}

// ListWorkflows handles GET /api/workflows
func (h *WorkflowHandler) ListWorkflows(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	role := middleware.RoleFromContext(c)

	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	scope := c.Query("scope") // "system", "user", or empty for all accessible
	workflowTypeQuery := strings.TrimSpace(c.Query("workflow_type"))
	sourceKindQuery := strings.TrimSpace(c.Query("source_kind"))
	targetKindQuery := strings.TrimSpace(c.Query("target_kind"))
	includeLegacy := c.DefaultQuery("include_legacy", "true") != "false"

	filter := appwf.WorkflowListFilter{IncludeLegacy: includeLegacy}
	if workflowTypeQuery != "" {
		value := domain.WorkflowType(workflowTypeQuery)
		filter.WorkflowType = &value
	}
	if sourceKindQuery != "" {
		value := domain.WorkflowSourceKind(sourceKindQuery)
		filter.SourceKind = &value
	}
	if targetKindQuery != "" {
		value := domain.WorkflowTargetKind(targetKindQuery)
		filter.TargetKind = &value
	}

	var result *appwf.WorkflowListResponse
	var err error

	switch scope {
	case "system":
		sysType := domain.OwnerSystem
		if filter.RequiresPrePagination() {
			result, err = h.service.ListWorkflowsFiltered(c.Request.Context(), &sysType, nil, role != "admin", offset, limit, filter)
		} else {
			result, err = h.service.ListWorkflows(c.Request.Context(), &sysType, nil, role != "admin", offset, limit)
		}
	case "user":
		userType := domain.OwnerUser
		if filter.RequiresPrePagination() {
			result, err = h.service.ListWorkflowsFiltered(c.Request.Context(), &userType, &userID, false, offset, limit, filter)
		} else {
			result, err = h.service.ListWorkflows(c.Request.Context(), &userType, &userID, false, offset, limit)
		}
	default:
		if role == "admin" {
			if filter.RequiresPrePagination() {
				result, err = h.service.ListWorkflowsFiltered(c.Request.Context(), nil, nil, false, offset, limit, filter)
			} else {
				result, err = h.service.ListWorkflows(c.Request.Context(), nil, nil, false, offset, limit)
			}
		} else {
			if filter.RequiresPrePagination() {
				result, err = h.service.ListUserAccessibleWorkflowsFiltered(c.Request.Context(), userID, offset, limit, filter)
			} else {
				result, err = h.service.ListUserAccessibleWorkflows(c.Request.Context(), userID, offset, limit)
			}
		}
	}

	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	if !filter.RequiresPrePagination() {
		result = h.service.FilterWorkflowList(result, filter)
	}
	response.Success(c, result)
}

// CreateWorkflow handles POST /api/workflows
func (h *WorkflowHandler) CreateWorkflow(c *gin.Context) {
	var req appwf.CreateWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	userID := middleware.UserIDFromContext(c)
	role := middleware.RoleFromContext(c)

	ownerType := domain.OwnerUser
	if req.OwnerType != nil && *req.OwnerType == domain.OwnerSystem {
		if role != "admin" {
			response.Error(c, http.StatusForbidden, errcode.CodeForbidden, "only admin can create system templates")
			return
		}
		ownerType = domain.OwnerSystem
	}

	result, err := h.service.CreateWorkflow(c.Request.Context(), ownerType, userID, &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, result)
}

// GetWorkflow handles GET /api/workflows/:id
func (h *WorkflowHandler) GetWorkflow(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid workflow id")
		return
	}

	result, err := h.service.GetWorkflow(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
		return
	}
	response.Success(c, result)
}

// UpdateWorkflow handles PUT /api/workflows/:id
func (h *WorkflowHandler) UpdateWorkflow(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid workflow id")
		return
	}

	var req appwf.UpdateWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	result, err := h.service.UpdateWorkflow(c.Request.Context(), id, &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, result)
}

// DeleteWorkflow handles DELETE /api/workflows/:id
func (h *WorkflowHandler) DeleteWorkflow(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid workflow id")
		return
	}

	if err := h.service.DeleteWorkflow(c.Request.Context(), id); err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, nil)
}

// BatchUpdateNodes handles PUT /api/workflows/:id/nodes
func (h *WorkflowHandler) BatchUpdateNodes(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid workflow id")
		return
	}

	var req appwf.BatchUpdateNodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	result, err := h.service.BatchUpdateNodes(c.Request.Context(), id, &req)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	response.Success(c, result)
}

// ExecuteWorkflow handles POST /api/workflows/:id/execute
func (h *WorkflowHandler) ExecuteWorkflow(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid workflow id")
		return
	}

	req, cleanup, err := h.bindExecuteWorkflowRequest(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	defer cleanup()

	inputText := req.InputText
	if req.AudioFilePath != "" {
		workflowResp, err := h.service.GetWorkflow(c.Request.Context(), id)
		if err != nil {
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
			return
		}
		if firstNodeType := firstEnabledNodeType(workflowResp.Nodes); firstNodeType != nil && firstNodeType.IsSource() {
			if h.asrService == nil {
				response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, "asr service is not configured")
				return
			}
			snippet, err := h.asrService.TranscribeSnippet(c.Request.Context(), &appasr.TranscribeSnippetRequest{LocalFilePath: req.AudioFilePath})
			if err != nil {
				if isASRBadRequest(err) {
					response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
					return
				}
				response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
				return
			}
			inputText = snippet.Text
		}
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.ExecuteWorkflow(
		c.Request.Context(), id,
		domain.TriggerManual, "",
		inputText,
		&wfengine.ExecutionMeta{UserID: userID, AudioURL: req.AudioURL, AudioFilePath: req.AudioFilePath},
	)
	if err != nil {
		// Still return the execution result even on failure
		if result != nil {
			response.Success(c, result)
			return
		}
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, result)
}

// CloneWorkflow handles POST /api/workflows/:id/clone
func (h *WorkflowHandler) CloneWorkflow(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid workflow id")
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.CloneWorkflow(c.Request.Context(), id, userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, result)
}

// TestNode handles POST /api/workflows/test-node
func (h *WorkflowHandler) TestNode(c *gin.Context) {
	req, cleanup, err := h.bindTestNodeRequest(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	defer cleanup()

	if wantsStreamNodeTest(c) {
		h.streamTestNode(c, &req)
		return
	}

	nodeType := domain.NodeType(req.NodeType)
	if nodeType.IsSource() && req.AudioFilePath != "" {
		if h.asrService == nil {
			response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, "asr service is not configured")
			return
		}
		snippet, err := h.asrService.TranscribeSnippet(c.Request.Context(), &appasr.TranscribeSnippetRequest{LocalFilePath: req.AudioFilePath})
		if err != nil {
			if isASRBadRequest(err) {
				response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
				return
			}
			response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
			return
		}
		response.Success(c, &appwf.TestNodeResponse{
			OutputText: snippet.Text,
			Detail:     json.RawMessage(fmt.Sprintf(`{"mode":"audio_source","status":%q,"duration":%v}`, snippet.Status, snippet.Duration)),
		})
		return
	}

	result, err := h.service.TestNode(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *WorkflowHandler) streamTestNode(c *gin.Context, req *appwf.TestNodeRequest) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, "streaming is not supported")
		return
	}

	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	writeEvent := func(event *appwf.TestNodeStreamEvent) error {
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := c.Writer.Write(append(payload, '\n')); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	nodeType := domain.NodeType(req.NodeType)
	if nodeType.IsSource() && req.AudioFilePath != "" {
		if h.asrService == nil {
			_ = writeEvent(&appwf.TestNodeStreamEvent{Type: "done", Error: "asr service is not configured"})
			return
		}
		snippet, err := h.asrService.TranscribeSnippet(c.Request.Context(), &appasr.TranscribeSnippetRequest{LocalFilePath: req.AudioFilePath})
		if err != nil {
			_ = writeEvent(&appwf.TestNodeStreamEvent{Type: "done", Error: err.Error()})
			return
		}
		detail, _ := json.Marshal(map[string]any{
			"mode":     "audio_source",
			"status":   snippet.Status,
			"duration": snippet.Duration,
		})
		_ = writeEvent(&appwf.TestNodeStreamEvent{
			Type:       "done",
			OutputText: snippet.Text,
			Detail:     detail,
		})
		return
	}

	if _, err := h.service.TestNodeStream(c.Request.Context(), req, writeEvent); err != nil {
		_ = writeEvent(&appwf.TestNodeStreamEvent{Type: "done", Error: err.Error()})
	}
}

func wantsStreamNodeTest(c *gin.Context) bool {
	if c.Query("stream") == "1" {
		return true
	}
	accept := strings.ToLower(strings.TrimSpace(c.GetHeader("Accept")))
	return strings.Contains(accept, "application/x-ndjson")
}

// GetNodeTypes handles GET /api/workflows/node-types
func (h *WorkflowHandler) GetNodeTypes(c *gin.Context) {
	types, err := h.service.GetNodeTypes(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, types)
}

// UpdateNodeDefault handles PUT /api/workflows/node-defaults/:nodeType
func (h *WorkflowHandler) UpdateNodeDefault(c *gin.Context) {
	nodeType := domain.NodeType(strings.TrimSpace(c.Param("nodeType")))
	var req appwf.UpdateNodeDefaultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	result, err := h.service.UpdateNodeDefault(c.Request.Context(), nodeType, &req)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	response.Success(c, result)
}

// GetExecution handles GET /api/workflow-executions/:id
func (h *WorkflowHandler) GetExecution(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid execution id")
		return
	}

	result, err := h.service.GetExecution(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *WorkflowHandler) bindExecuteWorkflowRequest(c *gin.Context) (appwf.ExecuteWorkflowRequest, func(), error) {
	if !isMultipartWorkflowRequest(c) {
		var req appwf.ExecuteWorkflowRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			return appwf.ExecuteWorkflowRequest{}, nil, err
		}
		return req, func() {}, nil
	}

	path, cleanup, err := saveOptionalWorkflowAudio(c, "workflow-execute")
	if err != nil {
		return appwf.ExecuteWorkflowRequest{}, cleanup, err
	}

	return appwf.ExecuteWorkflowRequest{
		InputText:     strings.TrimSpace(c.PostForm("input_text")),
		AudioURL:      strings.TrimSpace(c.PostForm("audio_url")),
		AudioFilePath: path,
	}, cleanup, nil
}

func (h *WorkflowHandler) bindTestNodeRequest(c *gin.Context) (appwf.TestNodeRequest, func(), error) {
	if !isMultipartWorkflowRequest(c) {
		var req appwf.TestNodeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			return appwf.TestNodeRequest{}, nil, err
		}
		return req, func() {}, nil
	}

	path, cleanup, err := saveOptionalWorkflowAudio(c, "workflow-test-node")
	if err != nil {
		return appwf.TestNodeRequest{}, cleanup, err
	}

	nodeType := strings.TrimSpace(c.PostForm("node_type"))
	if nodeType == "" {
		cleanup()
		return appwf.TestNodeRequest{}, func() {}, fmt.Errorf("node_type is required")
	}

	configText := strings.TrimSpace(c.PostForm("config"))
	if configText == "" {
		configText = "{}"
	}
	if !json.Valid([]byte(configText)) {
		cleanup()
		return appwf.TestNodeRequest{}, func() {}, fmt.Errorf("config must be valid JSON")
	}

	return appwf.TestNodeRequest{
		NodeType:      nodeType,
		Config:        json.RawMessage(configText),
		InputText:     strings.TrimSpace(c.PostForm("input_text")),
		AudioURL:      strings.TrimSpace(c.PostForm("audio_url")),
		AudioFilePath: path,
	}, cleanup, nil
}

func isMultipartWorkflowRequest(c *gin.Context) bool {
	return strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data")
}

func saveOptionalWorkflowAudio(c *gin.Context, prefix string) (string, func(), error) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return "", func() {}, nil
		}
		return "", func() {}, err
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !isSupportedAudioExtension(ext) {
		return "", func() {}, fmt.Errorf("unsupported audio file type")
	}

	absPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d%s", prefix, time.Now().UnixNano(), ext))
	src, err := fileHeader.Open()
	if err != nil {
		return "", func() {}, fmt.Errorf("failed to open uploaded audio file: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(absPath)
	if err != nil {
		return "", func() {}, fmt.Errorf("failed to create temp audio file: %w", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(absPath)
		return "", func() {}, fmt.Errorf("failed to save audio file: %w", err)
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(absPath)
		return "", func() {}, fmt.Errorf("failed to finalize audio file: %w", err)
	}

	return absPath, func() {
		_ = os.Remove(absPath)
	}, nil
}

func firstEnabledNodeType(nodes []appwf.NodeResponse) *domain.NodeType {
	var selected *domain.NodeType
	selectedPosition := 0
	for _, node := range nodes {
		if !node.Enabled {
			continue
		}
		if selected == nil || node.Position < selectedPosition {
			nodeType := node.NodeType
			selected = &nodeType
			selectedPosition = node.Position
		}
	}
	return selected
}
