package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	appasr "github.com/lgt/asr/internal/application/asr"
	appaudio "github.com/lgt/asr/internal/application/audio"
	appwf "github.com/lgt/asr/internal/application/workflow"
	domainasr "github.com/lgt/asr/internal/domain/asr"
	userdomain "github.com/lgt/asr/internal/domain/user"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
	"github.com/lgt/asr/internal/infrastructure/asrengine"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// ASRHandler exposes transcription task endpoints.
type ASRHandler struct {
	service        *appasr.Service
	audioService   *appaudio.Service
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
		maxAudioSizeMB = defaultMaxAudioSizeMB
	}

	return &ASRHandler{
		service:        service,
		audioService:   appaudio.NewService(service, nil),
		workflowSvc:    workflowSvc,
		uploadDir:      uploadDir,
		publicBaseURL:  strings.TrimRight(publicBaseURL, "/"),
		maxAudioSizeMB: maxAudioSizeMB,
	}
}

// Register registers ASR routes.
func (h *ASRHandler) Register(group *gin.RouterGroup) {
	group.POST("/tasks", h.CreateTask)
	group.DELETE("/tasks", h.ClearTasks)
	group.POST("/tasks/upload", h.UploadTaskFile)
	group.POST("/realtime-tasks/upload", h.UploadRealtimeTaskFile)
	group.POST("/stream-sessions", h.StartStreamSession)
	group.POST("/stream-sessions/:id/chunks", h.PushStreamChunk)
	group.POST("/stream-sessions/:id/commit", h.CommitStreamSession)
	group.POST("/stream-sessions/:id/finish", h.FinishStreamSession)
	group.POST("/realtime-segments", h.TranscribeRealtimeSegment)
	group.GET("/tasks", h.ListTasks)
	group.GET("/tasks/:id/executions", h.ListTaskExecutions)
	group.GET("/tasks/:id", h.GetTask)
	group.DELETE("/tasks/:id", h.DeleteTask)
	group.POST("/tasks/:id/resume-post-process", h.ResumeTaskPostProcess)
	group.POST("/tasks/:id/sync", h.SyncTask)
}

func (h *ASRHandler) CommitStreamSession(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Param("id"))
	if sessionID == "" {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid session id")
		return
	}

	result, err := h.service.CommitStreamSegment(c.Request.Context(), sessionID)
	if err != nil {
		if errors.Is(err, appasr.ErrStreamSessionNotFound) || errors.Is(err, appasr.ErrStreamSessionExpired) {
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *ASRHandler) StartStreamSession(c *gin.Context) {
	result, err := h.service.StartStreamSession(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *ASRHandler) PushStreamChunk(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Param("id"))
	if sessionID == "" {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid session id")
		return
	}

	maxBytes := maxAudioSizeBytes(h.maxAudioSizeMB)

	pcmData, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBytes+1))
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "failed to read audio chunk")
		return
	}
	if int64(len(pcmData)) > maxBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, errcode.CodeBadRequest, audioTooLargeMessage(h.maxAudioSizeMB))
		return
	}
	if len(pcmData) == 0 {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "missing audio chunk")
		return
	}

	result, err := h.service.PushStreamChunk(c.Request.Context(), &appasr.PushStreamChunkRequest{
		SessionID: sessionID,
		PCMData:   pcmData,
	})
	if err != nil {
		if errors.Is(err, appasr.ErrStreamSessionNotFound) || errors.Is(err, appasr.ErrStreamSessionExpired) {
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
			return
		}
		if errors.Is(err, appasr.ErrStreamSessionClosed) {
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *ASRHandler) FinishStreamSession(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Param("id"))
	if sessionID == "" {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid session id")
		return
	}

	result, err := h.service.FinishStreamSession(c.Request.Context(), sessionID)
	if err != nil {
		if errors.Is(err, appasr.ErrStreamSessionNotFound) || errors.Is(err, appasr.ErrStreamSessionExpired) {
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *ASRHandler) TranscribeRealtimeSegment(c *gin.Context) {
	audioFile, err := saveTemporaryUploadedAudio(c, "file", "asr-realtime", h.maxAudioSizeMB)
	if err != nil {
		status, messageText := resolveAudioUploadError(err)
		response.Error(c, status, errcode.CodeBadRequest, messageText)
		return
	}
	defer os.Remove(audioFile.AbsolutePath)

	var dictID *uint64
	if rawDictID := strings.TrimSpace(c.PostForm("dict_id")); rawDictID != "" {
		parsed, parseErr := strconv.ParseUint(rawDictID, 10, 64)
		if parseErr != nil {
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict_id")
			return
		}
		dictID = &parsed
	}

	var workflowID *uint64
	if rawWorkflowID := strings.TrimSpace(c.PostForm("workflow_id")); rawWorkflowID != "" {
		parsed, parseErr := strconv.ParseUint(rawWorkflowID, 10, 64)
		if parseErr != nil {
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid workflow_id")
			return
		}
		workflowID = &parsed
	}
	if dictID == nil {
		resolvedDictID, resolveErr := workflowTermDictID(c.Request.Context(), h.workflowSvc, workflowID)
		if resolveErr != nil {
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, resolveErr.Error())
			return
		}
		dictID = resolvedDictID
	}

	language, useITN, hotwords, err := parseASROptions(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	result, err := h.audioService.TranscribeRealtimeSegment(c.Request.Context(), appaudio.TranscribeRealtimeSegmentRequest{
		Audio: appaudio.PreparedAudio{
			OriginalFilename: audioFile.OriginalFilename,
			LocalFilePath:    audioFile.AbsolutePath,
			Duration:         audioFile.Duration,
		},
		DictID:   dictID,
		Language: language,
		UseITN:   useITN,
		Hotwords: hotwords,
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

	var dictID *uint64
	if rawDictID := strings.TrimSpace(c.PostForm("dict_id")); rawDictID != "" {
		parsed, err := strconv.ParseUint(rawDictID, 10, 64)
		if err != nil {
			_ = os.Remove(audioFile.AbsolutePath)
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict_id")
			return
		}
		dictID = &parsed
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
	if err := h.validateWorkflowBinding(c.Request.Context(), workflowID, domainasr.TaskTypeBatch); err != nil {
		_ = os.Remove(audioFile.AbsolutePath)
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	if dictID == nil {
		resolvedDictID, err := workflowTermDictID(c.Request.Context(), h.workflowSvc, workflowID)
		if err != nil {
			_ = os.Remove(audioFile.AbsolutePath)
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
			return
		}
		dictID = resolvedDictID
	}
	language, useITN, hotwords, err := parseASROptions(c)
	if err != nil {
		_ = os.Remove(audioFile.AbsolutePath)
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.audioService.CreateBatchTaskFromAudio(c.Request.Context(), userID, appaudio.CreateBatchTaskRequest{
		Audio: appaudio.PreparedAudio{
			OriginalFilename: audioFile.OriginalFilename,
			AudioURL:         audioURL,
			LocalFilePath:    audioFile.AbsolutePath,
			Duration:         audioFile.Duration,
		},
		DictID:     dictID,
		WorkflowID: workflowID,
		Language:   language,
		UseITN:     useITN,
		Hotwords:   hotwords,
	})
	if err != nil {
		_ = os.Remove(audioFile.AbsolutePath)
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
		"filename":  audioFile.OriginalFilename,
	})
}

func (h *ASRHandler) UploadRealtimeTaskFile(c *gin.Context) {
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

	resultText := strings.TrimSpace(c.PostForm("result_text"))
	if resultText == "" {
		_ = os.Remove(audioFile.AbsolutePath)
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "result_text is required")
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
	if err := h.validateWorkflowBinding(c.Request.Context(), workflowID, domainasr.TaskTypeRealtime); err != nil {
		_ = os.Remove(audioFile.AbsolutePath)
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	language, _, _, err := parseASROptions(c)
	if err != nil {
		_ = os.Remove(audioFile.AbsolutePath)
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.audioService.CreateRealtimeTaskFromAudio(c.Request.Context(), userID, appaudio.CreateRealtimeTaskRequest{
		Audio: appaudio.PreparedAudio{
			OriginalFilename: audioFile.OriginalFilename,
			AudioURL:         audioURL,
			LocalFilePath:    audioFile.AbsolutePath,
			Duration:         audioFile.Duration,
		},
		ResultText: resultText,
		WorkflowID: workflowID,
		Language:   language,
	})
	if err != nil {
		_ = os.Remove(audioFile.AbsolutePath)
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
		"filename":  audioFile.OriginalFilename,
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
	if req.DictID == nil && req.WorkflowID != nil {
		resolvedDictID, err := workflowTermDictID(c.Request.Context(), h.workflowSvc, req.WorkflowID)
		if err != nil {
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
			return
		}
		req.DictID = resolvedDictID
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.CreateTask(c.Request.Context(), userID, &req)
	if err != nil {
		if errors.Is(err, appasr.ErrStreamSessionNotFound) || errors.Is(err, appasr.ErrStreamSessionExpired) {
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
			return
		}
		if errors.Is(err, appasr.ErrStreamSessionActive) || errors.Is(err, appasr.ErrStreamSessionClosed) || errors.Is(err, appasr.ErrStreamSessionEmptyAudio) {
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
			return
		}
		if isASRBadRequest(err) {
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func requesterIsAdmin(c *gin.Context) bool {
	return middleware.RoleFromContext(c) == string(userdomain.RoleAdmin)
}

func (h *ASRHandler) ListTasks(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	taskType, err := parseOptionalTaskType(c.Query("type"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	userID := middleware.UserIDFromContext(c)

	var result *appasr.TaskListResponse
	if c.Query("scope") == "all" && requesterIsAdmin(c) {
		result, err = h.service.ListAllTasks(c.Request.Context(), taskType, offset, limit)
	} else {
		result, err = h.service.ListTasks(c.Request.Context(), userID, taskType, offset, limit)
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *ASRHandler) ClearTasks(c *gin.Context) {
	taskType, err := parseOptionalTaskType(c.Query("type"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.service.ClearTasks(c.Request.Context(), userID, taskType)
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
	var result *appasr.TaskResponse
	if requesterIsAdmin(c) {
		result, err = h.service.GetTaskAdmin(c.Request.Context(), id)
	} else {
		result, err = h.service.GetTask(c.Request.Context(), userID, id)
	}
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
	if requesterIsAdmin(c) {
		err = h.service.DeleteTaskAdmin(c.Request.Context(), id)
	} else {
		err = h.service.DeleteTask(c.Request.Context(), userID, id)
	}
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
	var result *appasr.TaskResponse
	if requesterIsAdmin(c) {
		result, err = h.service.AdminSyncTask(c.Request.Context(), id)
	} else {
		result, err = h.service.SyncTask(c.Request.Context(), userID, id)
	}
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
	var result *appasr.TaskResponse
	if requesterIsAdmin(c) {
		result, err = h.service.ResumeTaskPostProcessFromFailureAdmin(c.Request.Context(), id)
	} else {
		result, err = h.service.ResumeTaskPostProcessFromFailure(c.Request.Context(), userID, id)
	}
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
	if requesterIsAdmin(c) {
		if _, err := h.service.GetTaskAdmin(c.Request.Context(), id); err != nil {
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, err.Error())
			return
		}
	} else if _, err := h.service.GetTask(c.Request.Context(), userID, id); err != nil {
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

func isASRBadRequest(err error) bool {
	var upstreamErr *asrengine.UpstreamBadRequestError
	return errors.As(err, &upstreamErr)
}

func parseOptionalTaskType(raw string) (*domainasr.TaskType, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	taskType := domainasr.TaskType(trimmed)
	switch taskType {
	case domainasr.TaskTypeRealtime, domainasr.TaskTypeBatch:
		return &taskType, nil
	default:
		return nil, fmt.Errorf("invalid task type")
	}
}
