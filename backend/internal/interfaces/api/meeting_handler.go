package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	appaudio "github.com/lgt/asr/internal/application/audio"
	appmeeting "github.com/lgt/asr/internal/application/meeting"
	appwf "github.com/lgt/asr/internal/application/workflow"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
	"github.com/lgt/asr/internal/interfaces/middleware"
	pkgconfig "github.com/lgt/asr/pkg/config"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// MeetingHandler exposes meeting management endpoints.
type MeetingHandler struct {
	service        *appmeeting.Service
	audioService   *appaudio.Service
	workflowSvc    *appwf.Service
	uploadDir      string
	publicBaseURL  string
	maxAudioSizeMB int64
	feature        featureGate
}

// NewMeetingHandler creates a meeting handler.
func NewMeetingHandler(service *appmeeting.Service, workflowSvc *appwf.Service, uploadDir, publicBaseURL string, maxAudioSizeMB int64, features pkgconfig.ProductFeatures) *MeetingHandler {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "uploads"
	}
	if maxAudioSizeMB <= 0 {
		maxAudioSizeMB = 100
	}
	return &MeetingHandler{service: service, audioService: appaudio.NewService(nil, service), workflowSvc: workflowSvc, uploadDir: uploadDir, publicBaseURL: strings.TrimRight(publicBaseURL, "/"), maxAudioSizeMB: maxAudioSizeMB, feature: newFeatureGate(features)}
}

// Register registers meeting routes.
func (h *MeetingHandler) Register(group *gin.RouterGroup) {
	group.POST("/upload", h.Upload)
	group.POST("", h.Create)
	group.GET("", h.List)
	group.GET("/:id", h.Detail)
	group.PUT("/:id", h.Update)
	group.DELETE("/:id", h.Delete)
	group.POST("/:id/summary", h.RegenerateSummary)
}

func (h *MeetingHandler) Upload(c *gin.Context) {
	if !h.feature.meeting() {
		h.feature.denyFeature(c, "当前版本未开放会议纪要")
		return
	}
	audioFile, err := savePermanentUploadedAudio(c, "file", h.uploadDir, "audio", h.maxAudioSizeMB)
	if err != nil {
		status, messageText := resolveAudioUploadError(err)
		response.Error(c, status, errcode.CodeBadRequest, messageText)
		return
	}

	audioURL, err := buildUploadedFileURL(c, h.publicBaseURL, audioFile.RelativePath)
	if err != nil {
		_ = os.Remove(audioFile.AbsolutePath)
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	var workflowID *uint64
	if rawWorkflowID := strings.TrimSpace(c.PostForm("workflow_id")); rawWorkflowID != "" {
		parsed, err := strconv.ParseUint(rawWorkflowID, 10, 64)
		if err != nil {
			_ = os.Remove(audioFile.AbsolutePath)
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid workflow_id")
			return
		}
		workflowID = &parsed
	}
	if err := h.validateMeetingWorkflow(c.Request.Context(), workflowID); err != nil {
		_ = os.Remove(audioFile.AbsolutePath)
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	userID := middleware.UserIDFromContext(c)
	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		title = strings.TrimSpace(strings.TrimSuffix(audioFile.OriginalFilename, filepath.Ext(audioFile.OriginalFilename)))
	}
	result, err := h.audioService.CreateMeetingFromAudio(c.Request.Context(), userID, appaudio.CreateMeetingRequest{
		Audio: appaudio.PreparedAudio{
			OriginalFilename: audioFile.OriginalFilename,
			AudioURL:         audioURL,
			LocalFilePath:    audioFile.AbsolutePath,
			Duration:         audioFile.Duration,
		},
		Title:      title,
		WorkflowID: workflowID,
	})
	if err != nil {
		_ = os.Remove(audioFile.AbsolutePath)
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, gin.H{
		"meeting":   result,
		"audio_url": audioURL,
		"filename":  audioFile.OriginalFilename,
	})
}

func (h *MeetingHandler) Create(c *gin.Context) {
	if !h.feature.meeting() {
		h.feature.denyFeature(c, "当前版本未开放会议纪要")
		return
	}
	var req appmeeting.CreateMeetingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	if err := h.validateMeetingWorkflow(c.Request.Context(), req.WorkflowID); err != nil {
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
	if !h.feature.meeting() {
		h.feature.denyFeature(c, "当前版本未开放会议纪要")
		return
	}
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
	if !h.feature.meeting() {
		h.feature.denyFeature(c, "当前版本未开放会议纪要")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid meeting id")
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.GetMeetingForUser(c.Request.Context(), id, userID)
	if err != nil {
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *MeetingHandler) Delete(c *gin.Context) {
	if !h.feature.meeting() {
		h.feature.denyFeature(c, "当前版本未开放会议纪要")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid meeting id")
		return
	}

	userID := middleware.UserIDFromContext(c)
	err = h.service.DeleteMeeting(c.Request.Context(), id, userID)
	if err != nil {
		switch {
		case errors.Is(err, appmeeting.ErrMeetingDeleteNotAllowed):
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		case errors.Is(err, appmeeting.ErrMeetingNotFound):
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
		default:
			response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		}
		return
	}

	response.Success(c, gin.H{"deleted": true})
}

func (h *MeetingHandler) Update(c *gin.Context) {
	if !h.feature.meeting() {
		h.feature.denyFeature(c, "当前版本未开放会议纪要")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid meeting id")
		return
	}

	var req appmeeting.UpdateMeetingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.UpdateMeeting(c.Request.Context(), id, userID, &req)
	if err != nil {
		switch {
		case errors.Is(err, appmeeting.ErrMeetingNotFound):
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
		case errors.Is(err, appmeeting.ErrMeetingTitleRequired):
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		default:
			response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		}
		return
	}

	response.Success(c, result)
}

func (h *MeetingHandler) RegenerateSummary(c *gin.Context) {
	if !h.feature.meeting() {
		h.feature.denyFeature(c, "当前版本未开放会议纪要")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid meeting id")
		return
	}

	var req appmeeting.RegenerateSummaryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	if err := h.validateMeetingWorkflow(c.Request.Context(), req.WorkflowID); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.RegenerateSummary(c.Request.Context(), id, userID, &req)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *MeetingHandler) validateMeetingWorkflow(ctx context.Context, workflowID *uint64) error {
	if workflowID == nil {
		return nil
	}
	if h.workflowSvc == nil {
		return nil
	}
	_, err := h.workflowSvc.ValidateWorkflowBinding(ctx, *workflowID, wfdomain.WorkflowTypeMeeting)
	return err
}
