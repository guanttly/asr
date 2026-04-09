package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	appwf "github.com/lgt/asr/internal/application/workflow"
	domain "github.com/lgt/asr/internal/domain/workflow"
	wfengine "github.com/lgt/asr/internal/infrastructure/workflow"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// WorkflowHandler handles HTTP requests for workflow management.
type WorkflowHandler struct {
	service *appwf.Service
}

// NewWorkflowHandler creates a new workflow handler.
func NewWorkflowHandler(service *appwf.Service) *WorkflowHandler {
	return &WorkflowHandler{service: service}
}

// Register registers workflow routes on the given router group.
func (h *WorkflowHandler) Register(group *gin.RouterGroup) {
	wf := group.Group("/workflows")
	{
		wf.GET("", h.ListWorkflows)
		wf.POST("", h.CreateWorkflow)
		wf.GET("/node-types", h.GetNodeTypes)
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

	var req appwf.ExecuteWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.ExecuteWorkflow(
		c.Request.Context(), id,
		domain.TriggerManual, "",
		req.InputText,
		&wfengine.ExecutionMeta{UserID: userID, AudioURL: req.AudioURL},
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
	var req appwf.TestNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	result, err := h.service.TestNode(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, result)
}

// GetNodeTypes handles GET /api/workflows/node-types
func (h *WorkflowHandler) GetNodeTypes(c *gin.Context) {
	types := h.service.GetNodeTypes()
	response.Success(c, types)
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
