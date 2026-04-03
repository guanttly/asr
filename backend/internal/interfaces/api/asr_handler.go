package api

import (
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
	appasr "github.com/lgt/asr/internal/application/asr"
	domainasr "github.com/lgt/asr/internal/domain/asr"
	"github.com/lgt/asr/internal/infrastructure/asrengine"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// ASRHandler exposes transcription task endpoints.
type ASRHandler struct {
	service        *appasr.Service
	uploadDir      string
	publicBaseURL  string
	maxAudioSizeMB int64
}

// NewASRHandler creates an ASR handler.
func NewASRHandler(service *appasr.Service, uploadDir, publicBaseURL string, maxAudioSizeMB int64) *ASRHandler {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "uploads"
	}
	if maxAudioSizeMB <= 0 {
		maxAudioSizeMB = 100
	}

	return &ASRHandler{
		service:        service,
		uploadDir:      uploadDir,
		publicBaseURL:  strings.TrimRight(publicBaseURL, "/"),
		maxAudioSizeMB: maxAudioSizeMB,
	}
}

// Register registers ASR routes.
func (h *ASRHandler) Register(group *gin.RouterGroup) {
	group.POST("/tasks", h.CreateTask)
	group.POST("/tasks/upload", h.UploadTaskFile)
	group.GET("/tasks", h.ListTasks)
	group.GET("/tasks/:id", h.GetTask)
	group.POST("/tasks/:id/sync", h.SyncTask)
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

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.CreateTask(c.Request.Context(), userID, &appasr.CreateTaskRequest{
		AudioURL:      audioURL,
		LocalFilePath: absPath,
		Type:          domainasr.TaskTypeBatch,
		DictID:        dictID,
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
