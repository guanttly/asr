package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	appmeeting "github.com/lgt/asr/internal/application/meeting"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// MeetingHandler exposes meeting management endpoints.
type MeetingHandler struct {
	service *appmeeting.Service
}

// NewMeetingHandler creates a meeting handler.
func NewMeetingHandler(service *appmeeting.Service) *MeetingHandler {
	return &MeetingHandler{service: service}
}

// Register registers meeting routes.
func (h *MeetingHandler) Register(group *gin.RouterGroup) {
	group.POST("", h.Create)
	group.GET("", h.List)
	group.GET("/:id", h.Detail)
}

func (h *MeetingHandler) Create(c *gin.Context) {
	var req appmeeting.CreateMeetingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.CreateMeeting(c.Request.Context(), userID, &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *MeetingHandler) List(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	userID := middleware.UserIDFromContext(c)

	result, total, err := h.service.ListMeetings(c.Request.Context(), userID, offset, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, gin.H{"items": result, "total": total})
}

func (h *MeetingHandler) Detail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid meeting id")
		return
	}

	result, err := h.service.GetMeeting(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
		return
	}

	response.Success(c, result)
}
