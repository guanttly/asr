package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	appmeeting "github.com/lgt/asr/internal/application/meeting"
	appwf "github.com/lgt/asr/internal/application/workflow"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// MeetingHandler exposes meeting management endpoints.
type MeetingHandler struct {
	service        *appmeeting.Service
	workflowSvc    *appwf.Service
	uploadDir      string
	publicBaseURL  string
	maxAudioSizeMB int64
}

// NewMeetingHandler creates a meeting handler.
func NewMeetingHandler(service *appmeeting.Service, workflowSvc *appwf.Service, uploadDir, publicBaseURL string, maxAudioSizeMB int64) *MeetingHandler {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "uploads"
	}
	if maxAudioSizeMB <= 0 {
		maxAudioSizeMB = 100
	}
	return &MeetingHandler{service: service, workflowSvc: workflowSvc, uploadDir: uploadDir, publicBaseURL: strings.TrimRight(publicBaseURL, "/"), maxAudioSizeMB: maxAudioSizeMB}
}

// Register registers meeting routes.
func (h *MeetingHandler) Register(group *gin.RouterGroup) {
	group.POST("/upload", h.Upload)
	group.POST("", h.Create)
	group.GET("", h.List)
	group.GET("/:id", h.Detail)
	group.DELETE("/:id", h.Delete)
	group.POST("/:id/summary", h.RegenerateSummary)
}

func (h *MeetingHandler) Upload(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "missing audio file")
		return
	}

	maxBytes := h.maxAudioSizeMB * 1024 * 1024
	if maxBytes > 0 && fileHeader.Size > maxBytes {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, fmt.Sprintf("audio file exceeds %d MB limit", h.maxAudioSizeMB))
		return
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !isSupportedMeetingAudioExtension(ext) {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "unsupported audio file type")
		return
	}

	storedDir := filepath.Join(h.uploadDir, "audio")
	if err := os.MkdirAll(storedDir, 0o755); err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, "failed to prepare upload directory")
		return
	}

	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	absPath := filepath.Join(storedDir, filename)
	if err := c.SaveUploadedFile(fileHeader, absPath); err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, "failed to save audio file")
		return
	}

	audioURL, err := h.buildUploadedFileURL(c, path.Join("audio", filename))
	if err != nil {
		_ = os.Remove(absPath)
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	var workflowID *uint64
	if rawWorkflowID := strings.TrimSpace(c.PostForm("workflow_id")); rawWorkflowID != "" {
		parsed, err := strconv.ParseUint(rawWorkflowID, 10, 64)
		if err != nil {
			_ = os.Remove(absPath)
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid workflow_id")
			return
		}
		workflowID = &parsed
	}
	if err := h.validateMeetingWorkflow(c.Request.Context(), workflowID); err != nil {
		_ = os.Remove(absPath)
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	userID := middleware.UserIDFromContext(c)
	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		title = strings.TrimSpace(strings.TrimSuffix(fileHeader.Filename, filepath.Ext(fileHeader.Filename)))
	}
	result, err := h.service.CreateMeeting(c.Request.Context(), userID, &appmeeting.CreateMeetingRequest{
		Title:         title,
		AudioURL:      audioURL,
		LocalFilePath: absPath,
		WorkflowID:    workflowID,
	})
	if err != nil {
		_ = os.Remove(absPath)
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, gin.H{
		"meeting":   result,
		"audio_url": audioURL,
		"filename":  fileHeader.Filename,
	})
}

func (h *MeetingHandler) Create(c *gin.Context) {
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

func (h *MeetingHandler) Delete(c *gin.Context) {
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

func (h *MeetingHandler) RegenerateSummary(c *gin.Context) {
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

func (h *MeetingHandler) buildUploadedFileURL(c *gin.Context, relativePath string) (string, error) {
	baseURL := strings.TrimSpace(h.publicBaseURL)
	if baseURL == "" {
		baseURL = meetingPublicRequestBaseURL(c)
	}
	if baseURL == "" {
		return "", fmt.Errorf("unable to determine public upload base url")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid upload public base url")
	}
	parsed.Path = path.Join(parsed.Path, "/uploads", relativePath)
	return parsed.String(), nil
}

func meetingPublicRequestBaseURL(c *gin.Context) string {
	if origin := strings.TrimSpace(c.GetHeader("Origin")); origin != "" {
		if parsed, err := url.Parse(origin); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
		}
	}

	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		}
	}
	if scheme == "" {
		scheme = "http"
	}

	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = c.Request.Host
	}
	if host == "" {
		return ""
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

func isSupportedMeetingAudioExtension(ext string) bool {
	switch ext {
	case ".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".opus", ".webm":
		return true
	default:
		return false
	}
}
