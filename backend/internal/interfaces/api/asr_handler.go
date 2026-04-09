package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	appasr "github.com/lgt/asr/internal/application/asr"
	appwf "github.com/lgt/asr/internal/application/workflow"
	domainasr "github.com/lgt/asr/internal/domain/asr"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
	"github.com/lgt/asr/internal/infrastructure/asrengine"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// ASRHandler exposes transcription task endpoints.
type ASRHandler struct {
	service        *appasr.Service
	workflowSvc    *appwf.Service
	uploadDir      string
	publicBaseURL  string
	maxAudioSizeMB int64
}

// NewASRHandler creates an ASR handler.
func NewASRHandler(service *appasr.Service, workflowSvc *appwf.Service, uploadDir, publicBaseURL string, maxAudioSizeMB int64) *ASRHandler {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "uploads"
	}
	if maxAudioSizeMB <= 0 {
		maxAudioSizeMB = 100
	}

	return &ASRHandler{
		service:        service,
		workflowSvc:    workflowSvc,
		uploadDir:      uploadDir,
		publicBaseURL:  strings.TrimRight(publicBaseURL, "/"),
		maxAudioSizeMB: maxAudioSizeMB,
	}
}

// Register registers ASR routes.
func (h *ASRHandler) Register(group *gin.RouterGroup) {
	group.POST("/tasks", h.CreateTask)
	group.POST("/tasks/upload", h.UploadTaskFile)
	group.POST("/realtime-segments", h.TranscribeRealtimeSegment)
	group.GET("/tasks", h.ListTasks)
	group.GET("/tasks/:id/executions", h.ListTaskExecutions)
	group.GET("/tasks/:id", h.GetTask)
	group.DELETE("/tasks/:id", h.DeleteTask)
	group.POST("/tasks/:id/resume-post-process", h.ResumeTaskPostProcess)
	group.POST("/tasks/:id/sync", h.SyncTask)
}

func (h *ASRHandler) TranscribeRealtimeSegment(c *gin.Context) {
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
	if !isSupportedAudioExtension(ext) {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "unsupported audio file type")
		return
	}

	localPath := filepath.Join(os.TempDir(), fmt.Sprintf("asr-realtime-%d%s", time.Now().UnixNano(), ext))
	src, err := fileHeader.Open()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, "failed to read audio file")
		return
	}
	defer src.Close()

	dst, err := os.Create(localPath)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, "failed to prepare audio file")
		return
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		_ = os.Remove(localPath)
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, "failed to save audio file")
		return
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(localPath)
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, "failed to save audio file")
		return
	}
	defer os.Remove(localPath)

	var dictID *uint64
	if rawDictID := strings.TrimSpace(c.PostForm("dict_id")); rawDictID != "" {
		parsed, parseErr := strconv.ParseUint(rawDictID, 10, 64)
		if parseErr != nil {
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict_id")
			return
		}
		dictID = &parsed
	}

	result, err := h.service.TranscribeSnippet(c.Request.Context(), &appasr.TranscribeSnippetRequest{
		LocalFilePath: localPath,
		DictID:        dictID,
	})
	if err != nil {
		if isASRBadRequest(err) {
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *ASRHandler) UploadTaskFile(c *gin.Context) {
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
	if !isSupportedAudioExtension(ext) {
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

	var dictID *uint64
	if rawDictID := strings.TrimSpace(c.PostForm("dict_id")); rawDictID != "" {
		parsed, err := strconv.ParseUint(rawDictID, 10, 64)
		if err != nil {
			_ = os.Remove(absPath)
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict_id")
			return
		}
		dictID = &parsed
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
	if err := h.validateWorkflowBinding(c.Request.Context(), workflowID, domainasr.TaskTypeBatch); err != nil {
		_ = os.Remove(absPath)
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.CreateTask(c.Request.Context(), userID, &appasr.CreateTaskRequest{
		AudioURL:      audioURL,
		LocalFilePath: absPath,
		Type:          domainasr.TaskTypeBatch,
		DictID:        dictID,
		WorkflowID:    workflowID,
	})
	if err != nil {
		_ = os.Remove(absPath)
		if isASRBadRequest(err) {
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, gin.H{
		"task":      result,
		"audio_url": audioURL,
		"filename":  fileHeader.Filename,
	})
}

func (h *ASRHandler) CreateTask(c *gin.Context) {
	var req appasr.CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	if err := h.validateWorkflowBinding(c.Request.Context(), req.WorkflowID, req.Type); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.CreateTask(c.Request.Context(), userID, &req)
	if err != nil {
		if isASRBadRequest(err) {
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *ASRHandler) ListTasks(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	userID := middleware.UserIDFromContext(c)

	result, err := h.service.ListTasks(c.Request.Context(), userID, offset, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *ASRHandler) GetTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid task id")
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.GetTask(c.Request.Context(), userID, id)
	if err != nil {
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *ASRHandler) DeleteTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid task id")
		return
	}

	userID := middleware.UserIDFromContext(c)
	err = h.service.DeleteTask(c.Request.Context(), userID, id)
	if err != nil {
		switch {
		case errors.Is(err, appasr.ErrTaskDeleteNotAllowed):
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		case errors.Is(err, appasr.ErrTaskNotFound):
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
		default:
			response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		}
		return
	}

	response.Success(c, gin.H{"deleted": true})
}

func (h *ASRHandler) SyncTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid task id")
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.SyncTask(c.Request.Context(), userID, id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *ASRHandler) ResumeTaskPostProcess(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid task id")
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.ResumeTaskPostProcessFromFailure(c.Request.Context(), userID, id)
	if err != nil {
		switch {
		case errors.Is(err, appasr.ErrTaskResumeNotAllowed):
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		case errors.Is(err, appasr.ErrTaskNotFound):
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
		default:
			response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		}
		return
	}

	response.Success(c, result)
}

func (h *ASRHandler) ListTaskExecutions(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid task id")
		return
	}

	userID := middleware.UserIDFromContext(c)
	if _, err := h.service.GetTask(c.Request.Context(), userID, id); err != nil {
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
		return
	}

	if h.workflowSvc == nil {
		response.Success(c, []*appwf.ExecutionResponse{})
		return
	}

	items, err := h.workflowSvc.ListExecutionsByTask(c.Request.Context(), id, 0, 20)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, items)
}

func (h *ASRHandler) validateWorkflowBinding(ctx context.Context, workflowID *uint64, taskType domainasr.TaskType) error {
	if workflowID == nil {
		return nil
	}
	if h.workflowSvc == nil {
		return fmt.Errorf("workflow service unavailable")
	}

	expectedType := wfdomain.WorkflowTypeBatch
	if taskType == domainasr.TaskTypeRealtime {
		expectedType = wfdomain.WorkflowTypeRealtime
	}

	_, err := h.workflowSvc.ValidateWorkflowBinding(ctx, *workflowID, expectedType)
	return err
}

func (h *ASRHandler) buildUploadedFileURL(c *gin.Context, relativePath string) (string, error) {
	baseURL := strings.TrimSpace(h.publicBaseURL)
	if baseURL == "" {
		baseURL = publicRequestBaseURL(c)
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

func publicRequestBaseURL(c *gin.Context) string {
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

func isSupportedAudioExtension(ext string) bool {
	switch ext {
	case ".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".opus", ".webm":
		return true
	default:
		return false
	}
}

func isASRBadRequest(err error) bool {
	var upstreamErr *asrengine.UpstreamBadRequestError
	return errors.As(err, &upstreamErr)
}
