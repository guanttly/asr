package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	appasr "github.com/lgt/asr/internal/application/asr"
	userdomain "github.com/lgt/asr/internal/domain/user"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// DashboardHandler exposes admin dashboard overview endpoints.
type DashboardHandler struct {
	asrService    *appasr.Service
	warnThreshold int
	alertLimit    int
}

// NewDashboardHandler creates a dashboard handler.
func NewDashboardHandler(asrService *appasr.Service, warnThreshold, alertLimit int) *DashboardHandler {
	return &DashboardHandler{asrService: asrService, warnThreshold: warnThreshold, alertLimit: alertLimit}
}

// Register registers dashboard routes.
func (h *DashboardHandler) Register(group *gin.RouterGroup) {
	group.GET("/dashboard/overview", h.GetOverview)
	group.POST("/dashboard/tasks/:id/sync", h.SyncTask)
	group.POST("/dashboard/tasks/retry-post-process", h.RetryFailedPostProcess)
	group.POST("/dashboard/retry-history/delete-item", h.DeleteRetryHistoryItem)
	group.POST("/dashboard/retry-history/clear", h.ClearRetryHistory)
}

func (h *DashboardHandler) GetOverview(c *gin.Context) {
	if middleware.RoleFromContext(c) != string(userdomain.RoleAdmin) {
		response.Error(c, http.StatusForbidden, errcode.CodeForbidden, "admin role required")
		return
	}

	result, err := h.asrService.GetSyncHealth(c.Request.Context(), h.warnThreshold, h.alertLimit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *DashboardHandler) SyncTask(c *gin.Context) {
	if middleware.RoleFromContext(c) != string(userdomain.RoleAdmin) {
		response.Error(c, http.StatusForbidden, errcode.CodeForbidden, "admin role required")
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid task id")
		return
	}

	result, err := h.asrService.AdminSyncTask(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *DashboardHandler) RetryFailedPostProcess(c *gin.Context) {
	if middleware.RoleFromContext(c) != string(userdomain.RoleAdmin) {
		response.Error(c, http.StatusForbidden, errcode.CodeForbidden, "admin role required")
		return
	}

	var req appasr.RetryPostProcessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	result, err := h.asrService.AdminRetryFailedPostProcess(c.Request.Context(), limit, req.TaskIDs)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *DashboardHandler) ClearRetryHistory(c *gin.Context) {
	if middleware.RoleFromContext(c) != string(userdomain.RoleAdmin) {
		response.Error(c, http.StatusForbidden, errcode.CodeForbidden, "admin role required")
		return
	}

	result, err := h.asrService.AdminClearRetryHistory(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *DashboardHandler) DeleteRetryHistoryItem(c *gin.Context) {
	if middleware.RoleFromContext(c) != string(userdomain.RoleAdmin) {
		response.Error(c, http.StatusForbidden, errcode.CodeForbidden, "admin role required")
		return
	}

	var req appasr.DeleteRetryHistoryItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	createdAt, err := time.Parse(time.RFC3339, req.CreatedAt)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid created_at")
		return
	}

	result, err := h.asrService.AdminDeleteRetryHistoryItem(c.Request.Context(), createdAt)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}
