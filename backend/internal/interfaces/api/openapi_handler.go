package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	appasr "github.com/lgt/asr/internal/application/asr"
	appaudio "github.com/lgt/asr/internal/application/audio"
	appmeeting "github.com/lgt/asr/internal/application/meeting"
	appnlp "github.com/lgt/asr/internal/application/nlp"
	appopenplatform "github.com/lgt/asr/internal/application/openplatform"
	appwf "github.com/lgt/asr/internal/application/workflow"
	asrdomain "github.com/lgt/asr/internal/domain/asr"
	meetingdomain "github.com/lgt/asr/internal/domain/meeting"
	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
	"github.com/lgt/asr/internal/interfaces/middleware"
	pkgconfig "github.com/lgt/asr/pkg/config"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

var openStreamEventsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

type OpenAPIASRHandler struct {
	service        *appasr.Service
	audioService   *appaudio.Service
	openService    *appopenplatform.Service
	workflowSvc    *appwf.Service
	uploadDir      string
	publicBaseURL  string
	maxAudioSizeMB int64
}

func NewOpenAPIASRHandler(service *appasr.Service, workflowSvc *appwf.Service, openService *appopenplatform.Service, uploadDir, publicBaseURL string, maxAudioSizeMB int64) *OpenAPIASRHandler {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "uploads"
	}
	if maxAudioSizeMB <= 0 {
		maxAudioSizeMB = 1024
	}
	return &OpenAPIASRHandler{
		service:        service,
		audioService:   appaudio.NewService(service, nil),
		openService:    openService,
		workflowSvc:    workflowSvc,
		uploadDir:      uploadDir,
		publicBaseURL:  strings.TrimRight(publicBaseURL, "/"),
		maxAudioSizeMB: maxAudioSizeMB,
	}
}

func (h *OpenAPIASRHandler) Register(group *gin.RouterGroup) {
	group.POST("/recognize", h.Recognize)
	group.POST("/recognize/vad", h.RecognizeVAD)
	group.POST("/tasks", h.CreateTask)
	group.GET("/tasks/:task_id", h.GetTask)
	group.POST("/stream-sessions", h.StartStreamSession)
	group.POST("/stream-sessions/:id/chunks", h.PushStreamChunk)
	group.POST("/stream-sessions/:id/commit", h.CommitStreamSession)
	group.GET("/stream-sessions/:id/events", h.StreamSessionEvents)
	group.POST("/stream-sessions/:id/finish", h.FinishStreamSession)
}

func (h *OpenAPIASRHandler) Recognize(c *gin.Context) {
	h.createBatchRecognition(c, false, true)
}

func (h *OpenAPIASRHandler) RecognizeVAD(c *gin.Context) {
	h.createBatchRecognition(c, true, true)
}

func (h *OpenAPIASRHandler) CreateTask(c *gin.Context) {
	h.createBatchRecognition(c, false, false)
}

func (h *OpenAPIASRHandler) GetTask(c *gin.Context) {
	app := middleware.OpenAppFromContext(c)
	if app == nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, "open app context missing")
		return
	}
	taskID, err := parseOpenTaskID(c.Param("task_id"))
	if err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenValidation, "invalid task_id")
		return
	}
	result, err := h.service.GetTask(c.Request.Context(), openPlatformOwnerID(app), taskID)
	if err != nil {
		response.OpenError(c, http.StatusNotFound, errcode.OpenValidation, err.Error())
		return
	}
	response.OpenSuccess(c, h.openTaskPayload(c, result, nil, ""))
}

func (h *OpenAPIASRHandler) StartStreamSession(c *gin.Context) {
	result, err := h.service.StartStreamSession(c.Request.Context())
	if err != nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, err.Error())
		return
	}
	commitURL, eventsURL, wsURL := buildOpenStreamURLs(c, result.SessionID)
	response.OpenSuccess(c, gin.H{
		"request_id": middleware.RequestIDFromContext(c),
		"session_id": result.SessionID,
		"commit_url": commitURL,
		"events_url": eventsURL,
		"ws_url":     wsURL,
		"expires_at": time.Now().Add(15 * time.Minute).UTC().Format(time.RFC3339),
	})
}

func (h *OpenAPIASRHandler) PushStreamChunk(c *gin.Context) {
	pcmData, err := readOpenChunk(c, h.maxAudioSizeMB)
	if err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenValidation, err.Error())
		return
	}
	result, err := h.service.PushStreamChunk(c.Request.Context(), &appasr.PushStreamChunkRequest{SessionID: strings.TrimSpace(c.Param("id")), PCMData: pcmData})
	if err != nil {
		h.writeStreamError(c, err)
		return
	}
	response.OpenSuccess(c, gin.H{
		"request_id": middleware.RequestIDFromContext(c),
		"session_id": result.SessionID,
		"text":       result.Text,
		"text_delta": result.TextDelta,
		"is_final":   result.IsFinal,
		"language":   result.Language,
	})
}

func (h *OpenAPIASRHandler) CommitStreamSession(c *gin.Context) {
	result, err := h.service.CommitStreamSegment(c.Request.Context(), strings.TrimSpace(c.Param("id")))
	if err != nil {
		h.writeStreamError(c, err)
		return
	}
	response.OpenSuccess(c, gin.H{
		"request_id":   middleware.RequestIDFromContext(c),
		"session_id":   result.SessionID,
		"text":         result.Text,
		"text_delta":   result.TextDelta,
		"segment_text": result.TextDelta,
		"is_final":     result.IsFinal,
		"language":     result.Language,
	})
}

func (h *OpenAPIASRHandler) StreamSessionEvents(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Param("id"))
	if sessionID == "" {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenValidation, "invalid session id")
		return
	}

	conn, err := openStreamEventsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	_ = conn.WriteJSON(gin.H{
		"type":       "session.ready",
		"request_id": middleware.RequestIDFromContext(c),
		"session_id": sessionID,
		"ts":         time.Now().UTC().Format(time.RFC3339),
	})

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	lastText := ""
	lastCommittedText := ""
	for {
		state, stateErr := h.service.GetStreamSessionState(c.Request.Context(), sessionID)
		if stateErr != nil {
			_ = conn.WriteJSON(gin.H{
				"type":       "session.error",
				"request_id": middleware.RequestIDFromContext(c),
				"session_id": sessionID,
				"code":       errcode.OpenSessionExpired,
				"message":    stateErr.Error(),
				"ts":         time.Now().UTC().Format(time.RFC3339),
			})
			return
		}

		if state.Text != lastText {
			_ = conn.WriteJSON(gin.H{
				"type":        "transcript.partial",
				"request_id":  middleware.RequestIDFromContext(c),
				"session_id":  state.SessionID,
				"language":    state.Language,
				"text":        state.Text,
				"text_delta":  openStreamTextDelta(lastText, state.Text),
				"duration_ms": int(math.Round(state.Duration * 1000)),
				"is_final":    state.IsFinal,
				"ts":          time.Now().UTC().Format(time.RFC3339),
			})
			lastText = state.Text
		}

		if state.CommittedText != lastCommittedText {
			segmentText := openStreamTextDelta(lastCommittedText, state.CommittedText)
			_ = conn.WriteJSON(gin.H{
				"type":          "transcript.segment",
				"request_id":    middleware.RequestIDFromContext(c),
				"session_id":    state.SessionID,
				"language":      state.Language,
				"text":          state.CommittedText,
				"segment_text":  segmentText,
				"text_delta":    segmentText,
				"duration_ms":   int(math.Round(state.Duration * 1000)),
				"segment_count": len(splitOpenTranscriptSegments(state.CommittedText)),
				"is_final":      state.IsFinal,
				"ts":            time.Now().UTC().Format(time.RFC3339),
			})
			lastCommittedText = state.CommittedText
		}

		if state.IsFinal {
			_ = conn.WriteJSON(gin.H{
				"type":        "session.finished",
				"request_id":  middleware.RequestIDFromContext(c),
				"session_id":  state.SessionID,
				"language":    state.Language,
				"text":        state.Text,
				"duration_ms": int(math.Round(state.Duration * 1000)),
				"is_final":    true,
				"ts":          time.Now().UTC().Format(time.RFC3339),
			})
			return
		}

		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *OpenAPIASRHandler) FinishStreamSession(c *gin.Context) {
	result, err := h.service.FinishStreamSession(c.Request.Context(), strings.TrimSpace(c.Param("id")))
	if err != nil {
		h.writeStreamError(c, err)
		return
	}
	response.OpenSuccess(c, gin.H{
		"request_id": middleware.RequestIDFromContext(c),
		"session_id": result.SessionID,
		"text":       result.Text,
		"text_delta": result.TextDelta,
		"is_final":   true,
		"language":   result.Language,
	})
}

func (h *OpenAPIASRHandler) createBatchRecognition(c *gin.Context, withVAD bool, wait bool) {
	app := middleware.OpenAppFromContext(c)
	if app == nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, "open app context missing")
		return
	}
	audioFile, err := savePermanentUploadedAudio(c, "file", h.uploadDir, "audio", h.maxAudioSizeMB)
	if err != nil {
		status, messageText := resolveAudioUploadError(err)
		response.OpenError(c, status, errcode.OpenValidation, messageText)
		return
	}
	audioURL, err := buildUploadedFileURL(c, h.publicBaseURL, audioFile.RelativePath)
	if err != nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, err.Error())
		return
	}
	if wait && audioFile.Duration > 60 {
		response.OpenError(c, http.StatusUnprocessableEntity, errcode.OpenAudioTooLong, "audio duration exceeds 60 seconds for synchronous recognition")
		return
	}
	workflowID, origin, err := resolveOpenWorkflow(c.Request.Context(), h.workflowSvc, app, "asr.recognize", c.PostForm("workflow_id"))
	if err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenWorkflowInvalid, err.Error())
		return
	}
	result, err := h.audioService.CreateBatchTaskFromAudio(c.Request.Context(), openPlatformOwnerID(app), appaudio.CreateBatchTaskRequest{
		Audio: appaudio.PreparedAudio{
			OriginalFilename: audioFile.OriginalFilename,
			AudioURL:         audioURL,
			LocalFilePath:    audioFile.AbsolutePath,
			Duration:         audioFile.Duration,
		},
		WorkflowID: workflowID,
	})
	if err != nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, err.Error())
		return
	}
	callbackURL := strings.TrimSpace(c.PostForm("callback_url"))
	if !wait || callbackURL != "" {
		if callbackURL != "" {
			h.dispatchTaskCallback(app, callbackURL, middleware.RequestIDFromContext(c), result.ID, workflowID, origin, withVAD)
		}
		payload := gin.H{
			"request_id":   middleware.RequestIDFromContext(c),
			"task_id":      openTaskID(result.ID),
			"status":       toOpenTaskStatus(result.Status),
			"callback_url": callbackURL,
		}
		if audioFile.Duration > 0 {
			payload["estimated_duration_sec"] = int(math.Ceil(audioFile.Duration))
		}
		response.OpenSuccess(c, payload)
		return
	}
	completed, err := h.waitForTask(c.Request.Context(), app, result.ID, 65*time.Second)
	if err != nil {
		response.OpenError(c, http.StatusGatewayTimeout, errcode.OpenInternal, err.Error())
		return
	}
	payload := gin.H{
		"request_id":  middleware.RequestIDFromContext(c),
		"duration_ms": int(math.Round(completed.Duration * 1000)),
		"language":    formOrDefault(c, "language", "auto"),
		"text":        completed.ResultText,
		"segments":    buildOpenTranscriptPayload(completed.ResultText, completed.Duration, withVAD),
		"task_id":     openTaskID(completed.ID),
	}
	if workflowID != nil {
		payload["workflow_id"] = *workflowID
		payload["workflow_origin"] = origin
		payload["post_processed_text"] = completed.ResultText
	}
	if withVAD {
		payload["vad_enabled"] = true
	}
	response.OpenSuccess(c, payload)
}

func (h *OpenAPIASRHandler) dispatchTaskCallback(app *openplatformdomain.App, callbackURL, requestID string, taskID uint64, workflowID *uint64, workflowOrigin string, withVAD bool) {
	if h.openService == nil || app == nil || strings.TrimSpace(callbackURL) == "" {
		return
	}
	appCopy := *app
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()

		payload := gin.H{
			"request_id": requestID,
			"task_id":    openTaskID(taskID),
			"status":     "failed",
			"error":      gin.H{"code": errcode.OpenInternal, "message": "task callback failed"},
		}
		result, err := h.waitForTask(ctx, &appCopy, taskID, 90*time.Minute)
		if err == nil {
			payload = h.openTaskPayloadWithRequestID(requestID, result, workflowID, workflowOrigin, withVAD)
		} else {
			payload["error"] = gin.H{"code": errcode.OpenInternal, "message": err.Error()}
		}
		_, _, _ = h.openService.DispatchOpenCallback(ctx, &appCopy, callbackURL, payload, map[string]string{
			"X-OpenAPI-Request-Id": requestID,
		})
	}()
}

func (h *OpenAPIASRHandler) waitForTask(ctx context.Context, app *openplatformdomain.App, taskID uint64, timeout time.Duration) (*appasr.TaskResponse, error) {
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		result, err := h.service.GetTask(pollCtx, openPlatformOwnerID(app), taskID)
		if err != nil {
			return nil, err
		}
		if result.Status == asrdomain.TaskStatusCompleted || result.Status == asrdomain.TaskStatusFailed {
			return result, nil
		}
		select {
		case <-pollCtx.Done():
			return nil, fmt.Errorf("task %s timed out", openTaskID(taskID))
		case <-ticker.C:
		}
	}
}

func (h *OpenAPIASRHandler) openTaskPayload(c *gin.Context, result *appasr.TaskResponse, workflowID *uint64, workflowOrigin string) gin.H {
	return h.openTaskPayloadWithRequestID(middleware.RequestIDFromContext(c), result, workflowID, workflowOrigin, false)
}

func (h *OpenAPIASRHandler) openTaskPayloadWithRequestID(requestID string, result *appasr.TaskResponse, workflowID *uint64, workflowOrigin string, withVAD bool) gin.H {
	payload := gin.H{
		"request_id": requestID,
		"task_id":    openTaskID(result.ID),
		"status":     toOpenTaskStatus(result.Status),
		"progress":   float64(result.ProgressPercent) / 100,
	}
	if result.Status == asrdomain.TaskStatusCompleted {
		payload["data"] = gin.H{
			"text":        result.ResultText,
			"duration_ms": int(math.Round(result.Duration * 1000)),
			"segments":    buildOpenTranscriptPayload(result.ResultText, result.Duration, withVAD),
		}
	}
	if result.Status == asrdomain.TaskStatusFailed {
		payload["error"] = gin.H{"code": errcode.OpenInternal, "message": strings.TrimSpace(result.LastSyncError)}
	}
	if workflowID != nil {
		payload["workflow_id"] = *workflowID
		payload["workflow_origin"] = workflowOrigin
	}
	if withVAD {
		payload["vad_enabled"] = true
	}
	return payload
}

func (h *OpenAPIASRHandler) writeStreamError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appasr.ErrStreamSessionNotFound):
		response.OpenError(c, http.StatusNotFound, errcode.OpenSessionExpired, err.Error())
	case errors.Is(err, appasr.ErrStreamSessionExpired):
		response.OpenError(c, http.StatusGone, errcode.OpenSessionExpired, err.Error())
	case errors.Is(err, appasr.ErrStreamSessionClosed):
		response.OpenError(c, http.StatusBadRequest, errcode.OpenValidation, err.Error())
	default:
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, err.Error())
	}
}

type OpenAPIMeetingHandler struct {
	service        *appmeeting.Service
	audioService   *appaudio.Service
	nlpService     *appnlp.Service
	openService    *appopenplatform.Service
	workflowSvc    *appwf.Service
	uploadDir      string
	publicBaseURL  string
	maxAudioSizeMB int64
	feature        featureGate
}

func NewOpenAPIMeetingHandler(service *appmeeting.Service, nlpService *appnlp.Service, workflowSvc *appwf.Service, openService *appopenplatform.Service, uploadDir, publicBaseURL string, maxAudioSizeMB int64, features pkgconfig.ProductFeatures) *OpenAPIMeetingHandler {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "uploads"
	}
	if maxAudioSizeMB <= 0 {
		maxAudioSizeMB = 1024
	}
	return &OpenAPIMeetingHandler{
		service:        service,
		audioService:   appaudio.NewService(nil, service),
		nlpService:     nlpService,
		openService:    openService,
		workflowSvc:    workflowSvc,
		uploadDir:      uploadDir,
		publicBaseURL:  strings.TrimRight(publicBaseURL, "/"),
		maxAudioSizeMB: maxAudioSizeMB,
		feature:        newFeatureGate(features),
	}
}

func (h *OpenAPIMeetingHandler) Register(group *gin.RouterGroup) {
	group.POST("/audio-summary", h.AudioSummary)
	group.POST("/text-summary", h.TextSummary)
	group.GET("/templates", h.Templates)
	group.GET("/:id", h.GetMeeting)
	group.POST("/:id/regenerate-summary", h.RegenerateSummary)
}

func (h *OpenAPIMeetingHandler) AudioSummary(c *gin.Context) {
	if !h.feature.meeting() {
		response.OpenError(c, http.StatusForbidden, errcode.OpenEditionLimited, "current edition does not support meeting summary")
		return
	}
	app := middleware.OpenAppFromContext(c)
	if app == nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, "open app context missing")
		return
	}
	audioFile, err := savePermanentUploadedAudio(c, "audio_file", h.uploadDir, "audio", h.maxAudioSizeMB)
	if err != nil {
		status, messageText := resolveAudioUploadError(err)
		response.OpenError(c, status, errcode.OpenValidation, messageText)
		return
	}
	audioURL, err := buildUploadedFileURL(c, h.publicBaseURL, audioFile.RelativePath)
	if err != nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, err.Error())
		return
	}
	workflowID, workflowOrigin, err := resolveOpenWorkflow(c.Request.Context(), h.workflowSvc, app, "meeting.summary", c.PostForm("workflow_id"))
	if err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenWorkflowInvalid, err.Error())
		return
	}
	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		title = strings.TrimSpace(strings.TrimSuffix(audioFile.OriginalFilename, "."+filepathExt(audioFile.OriginalFilename)))
	}
	meeting, err := h.audioService.CreateMeetingFromAudio(c.Request.Context(), openPlatformOwnerID(app), appaudio.CreateMeetingRequest{
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
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, err.Error())
		return
	}
	callbackURL := strings.TrimSpace(c.PostForm("callback_url"))
	if callbackURL != "" {
		h.dispatchMeetingCallback(app, callbackURL, middleware.RequestIDFromContext(c), meeting.ID, workflowID, workflowOrigin)
		response.OpenSuccess(c, gin.H{
			"request_id":   middleware.RequestIDFromContext(c),
			"meeting_id":   meeting.ID,
			"task_id":      fmt.Sprintf("mtask_%d", meeting.ID),
			"status":       meeting.Status,
			"callback_url": callbackURL,
		})
		return
	}
	completed, err := h.waitForMeeting(c.Request.Context(), app, meeting.ID, 5*time.Minute)
	if err != nil {
		response.OpenError(c, http.StatusGatewayTimeout, errcode.OpenInternal, err.Error())
		return
	}
	payload, payloadErr := h.buildMeetingPayload(c.Request.Context(), middleware.RequestIDFromContext(c), completed)
	if payloadErr != nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, payloadErr.Error())
		return
	}
	response.OpenSuccess(c, payload)
}

func (h *OpenAPIMeetingHandler) TextSummary(c *gin.Context) {
	if !h.feature.meeting() {
		response.OpenError(c, http.StatusForbidden, errcode.OpenEditionLimited, "current edition does not support meeting summary")
		return
	}
	var req struct {
		Text string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenValidation, err.Error())
		return
	}
	result, err := h.nlpService.Summarize(c.Request.Context(), &appnlp.SummarizeRequest{Text: req.Text})
	if err != nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, err.Error())
		return
	}
	response.OpenSuccess(c, gin.H{
		"request_id": middleware.RequestIDFromContext(c),
		"summary": gin.H{
			"title":    "会议纪要",
			"abstract": firstNonEmptyLine(result.Content),
			"raw_text": result.Content,
		},
	})
}

func (h *OpenAPIMeetingHandler) Templates(c *gin.Context) {
	response.OpenSuccess(c, gin.H{
		"default_template": "default",
		"templates": []gin.H{{
			"name":         "default",
			"display_name": "通用纪要",
			"variables":    []string{},
		}},
	})
}

func (h *OpenAPIMeetingHandler) GetMeeting(c *gin.Context) {
	app := middleware.OpenAppFromContext(c)
	if app == nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, "open app context missing")
		return
	}
	meetingID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenValidation, "invalid meeting id")
		return
	}
	result, err := h.service.GetMeetingForUser(c.Request.Context(), meetingID, openPlatformOwnerID(app))
	if err != nil {
		response.OpenError(c, http.StatusNotFound, errcode.OpenValidation, err.Error())
		return
	}
	payload, payloadErr := h.buildMeetingPayload(c.Request.Context(), middleware.RequestIDFromContext(c), result)
	if payloadErr != nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, payloadErr.Error())
		return
	}
	response.OpenSuccess(c, payload)
}

func (h *OpenAPIMeetingHandler) RegenerateSummary(c *gin.Context) {
	app := middleware.OpenAppFromContext(c)
	if app == nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, "open app context missing")
		return
	}
	meetingID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenValidation, "invalid meeting id")
		return
	}
	workflowID, _, err := resolveOpenWorkflow(c.Request.Context(), h.workflowSvc, app, "meeting.summary", queryOrBodyWorkflowID(c))
	if err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenWorkflowInvalid, err.Error())
		return
	}
	result, err := h.service.RegenerateSummary(c.Request.Context(), meetingID, openPlatformOwnerID(app), &appmeeting.RegenerateSummaryRequest{WorkflowID: workflowID})
	if err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenWorkflowInvalid, err.Error())
		return
	}
	payload, payloadErr := h.buildMeetingPayload(c.Request.Context(), middleware.RequestIDFromContext(c), result)
	if payloadErr != nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, payloadErr.Error())
		return
	}
	response.OpenSuccess(c, payload)
}

func (h *OpenAPIMeetingHandler) waitForMeeting(ctx context.Context, app *openplatformdomain.App, meetingID uint64, timeout time.Duration) (*appmeeting.MeetingDetailResponse, error) {
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		result, err := h.service.GetMeetingForUser(pollCtx, meetingID, openPlatformOwnerID(app))
		if err != nil {
			return nil, err
		}
		if result.Status == string(meetingdomain.MeetingStatusCompleted) || result.Status == string(meetingdomain.MeetingStatusFailed) {
			return result, nil
		}
		select {
		case <-pollCtx.Done():
			return nil, fmt.Errorf("meeting %d timed out", meetingID)
		case <-ticker.C:
		}
	}
}

func (h *OpenAPIMeetingHandler) dispatchMeetingCallback(app *openplatformdomain.App, callbackURL, requestID string, meetingID uint64, workflowID *uint64, workflowOrigin string) {
	if h.openService == nil || app == nil || strings.TrimSpace(callbackURL) == "" {
		return
	}
	appCopy := *app
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()

		payload := gin.H{
			"request_id": requestID,
			"meeting_id": meetingID,
			"task_id":    fmt.Sprintf("mtask_%d", meetingID),
			"status":     string(meetingdomain.MeetingStatusFailed),
			"error":      gin.H{"code": errcode.OpenInternal, "message": "meeting callback failed"},
		}
		result, err := h.waitForMeeting(ctx, &appCopy, meetingID, 90*time.Minute)
		if err == nil {
			payload = gin.H{
				"request_id": requestID,
				"meeting_id": result.ID,
				"task_id":    fmt.Sprintf("mtask_%d", result.ID),
				"status":     result.Status,
			}
			if workflowID != nil {
				payload["workflow_id"] = *workflowID
				payload["workflow_origin"] = workflowOrigin
			}
			if result.Status == string(meetingdomain.MeetingStatusCompleted) {
				data, payloadErr := h.buildMeetingPayload(ctx, requestID, result)
				if payloadErr == nil {
					payload["data"] = data
				} else {
					payload["status"] = string(meetingdomain.MeetingStatusFailed)
					payload["error"] = gin.H{"code": errcode.OpenInternal, "message": payloadErr.Error()}
				}
			} else {
				payload["error"] = gin.H{"code": errcode.OpenInternal, "message": strings.TrimSpace(result.LastSyncError)}
			}
		} else {
			payload["error"] = gin.H{"code": errcode.OpenInternal, "message": err.Error()}
		}
		_, _, _ = h.openService.DispatchOpenCallback(ctx, &appCopy, callbackURL, payload, map[string]string{
			"X-OpenAPI-Request-Id": requestID,
		})
	}()
}

func (h *OpenAPIMeetingHandler) buildMeetingPayload(ctx context.Context, requestID string, result *appmeeting.MeetingDetailResponse) (gin.H, error) {
	transcriptText := buildTranscriptText(result.Transcripts)
	summaryText := summaryContent(result)
	if summaryText == "" && h.nlpService != nil && strings.TrimSpace(transcriptText) != "" {
		summary, err := h.nlpService.Summarize(ctx, &appnlp.SummarizeRequest{Text: transcriptText})
		if err != nil {
			return nil, err
		}
		summaryText = summary.Content
	}
	return gin.H{
		"request_id": requestID,
		"meeting_id": result.ID,
		"asr": gin.H{
			"text":         transcriptText,
			"duration_sec": result.Duration,
			"language":     "auto",
		},
		"summary": gin.H{
			"title":    result.Title,
			"abstract": firstNonEmptyLine(summaryText),
			"raw_text": summaryText,
		},
	}, nil
}

type OpenAPINLPHandler struct {
	service *appnlp.Service
}

func NewOpenAPINLPHandler(service *appnlp.Service) *OpenAPINLPHandler {
	return &OpenAPINLPHandler{service: service}
}

func (h *OpenAPINLPHandler) Register(group *gin.RouterGroup) {
	group.POST("/correct", h.Correct)
}

func (h *OpenAPINLPHandler) Correct(c *gin.Context) {
	var req appnlp.CorrectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.OpenError(c, http.StatusBadRequest, errcode.OpenValidation, err.Error())
		return
	}
	result, err := h.service.Correct(c.Request.Context(), &req)
	if err != nil {
		response.OpenError(c, http.StatusInternalServerError, errcode.OpenInternal, err.Error())
		return
	}
	response.OpenSuccess(c, gin.H{
		"request_id":     middleware.RequestIDFromContext(c),
		"original_text":  result.OriginalText,
		"corrected_text": result.CorrectedText,
		"corrections":    result.Corrections,
	})
}

func resolveOpenWorkflow(ctx context.Context, workflowSvc *appwf.Service, app *openplatformdomain.App, capability, rawWorkflowID string) (*uint64, string, error) {
	trimmed := strings.TrimSpace(rawWorkflowID)
	if trimmed != "" {
		workflowID, err := strconv.ParseUint(trimmed, 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("invalid workflow_id")
		}
		if err := validateOpenWorkflow(ctx, workflowSvc, capability, workflowID); err != nil {
			return nil, "", err
		}
		return &workflowID, "request", nil
	}
	if app != nil && app.DefaultWorkflows != nil {
		if workflowID, ok := app.DefaultWorkflows[capability]; ok && workflowID > 0 {
			if err := validateOpenWorkflow(ctx, workflowSvc, capability, workflowID); err != nil {
				return nil, "", err
			}
			return &workflowID, "app_default", nil
		}
	}
	return nil, "", nil
}

func validateOpenWorkflow(ctx context.Context, workflowSvc *appwf.Service, capability string, workflowID uint64) error {
	if workflowSvc == nil || workflowID == 0 {
		return nil
	}
	workflowType, ok := workflowTypeForOpenCapability(capability)
	if !ok {
		return fmt.Errorf("capability %s does not support workflows", capability)
	}
	_, err := workflowSvc.ValidateWorkflowBinding(ctx, workflowID, workflowType)
	return err
}

func workflowTypeForOpenCapability(capability string) (wfdomain.WorkflowType, bool) {
	switch capability {
	case "asr.recognize":
		return wfdomain.WorkflowTypeBatch, true
	case "meeting.summary":
		return wfdomain.WorkflowTypeMeeting, true
	default:
		return "", false
	}
}

func openPlatformOwnerID(app *openplatformdomain.App) uint64 {
	if app == nil {
		return 0
	}
	return openplatformdomain.OwnerUserIDForApp(app.ID)
}

func openTaskID(id uint64) string {
	return fmt.Sprintf("task_%d", id)
}

func parseOpenTaskID(raw string) (uint64, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "task_")
	return strconv.ParseUint(trimmed, 10, 64)
}

func toOpenTaskStatus(status asrdomain.TaskStatus) string {
	switch status {
	case asrdomain.TaskStatusCompleted:
		return "succeeded"
	case asrdomain.TaskStatusFailed:
		return "failed"
	case asrdomain.TaskStatusProcessing:
		return "running"
	default:
		return "pending"
	}
}

func readOpenChunk(c *gin.Context, maxAudioSizeMB int64) ([]byte, error) {
	maxBytes := maxAudioSizeMB * 1024 * 1024
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	chunk, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read audio chunk")
	}
	if int64(len(chunk)) > maxBytes {
		return nil, fmt.Errorf("audio chunk exceeds size limit")
	}
	if len(chunk) == 0 {
		return nil, fmt.Errorf("missing audio chunk")
	}
	return chunk, nil
}

func formOrDefault(c *gin.Context, key, fallback string) string {
	value := strings.TrimSpace(c.PostForm(key))
	if value == "" {
		return fallback
	}
	return value
}

func buildOpenStreamURLs(c *gin.Context, sessionID string) (string, string, string) {
	baseURL := publicRequestBaseURL(c)
	if baseURL == "" {
		return "", "", ""
	}
	basePath := strings.TrimRight(baseURL, "/") + "/openapi/v1/asr/stream-sessions/" + sessionID
	commitURL := basePath + "/commit"
	eventsURL := basePath + "/events"
	if token := openAccessTokenFromRequest(c); token != "" {
		query := url.Values{"access_token": []string{token}}.Encode()
		eventsURL += "?" + query
		commitURL += "?" + query
	}
	wsURL := strings.Replace(eventsURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	return commitURL, eventsURL, wsURL
}

func openAccessTokenFromRequest(c *gin.Context) string {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if header != "" {
		if strings.HasPrefix(strings.ToLower(header), "bearer ") {
			return strings.TrimSpace(header[7:])
		}
		return header
	}
	return strings.TrimSpace(c.Query("access_token"))
}

func buildOpenTranscriptPayload(text string, duration float64, withVAD bool) []gin.H {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	parts := []string{trimmed}
	if withVAD {
		parts = splitOpenTranscriptSegments(trimmed)
		if len(parts) == 0 {
			parts = []string{trimmed}
		}
	}
	totalRunes := 0
	runeCounts := make([]int, len(parts))
	for index, part := range parts {
		runeCounts[index] = len([]rune(strings.TrimSpace(part)))
		if runeCounts[index] == 0 {
			runeCounts[index] = 1
		}
		totalRunes += runeCounts[index]
	}
	durationMs := int(math.Round(duration * 1000))
	segments := make([]gin.H, 0, len(parts))
	consumedRunes := 0
	startMs := 0
	for index, part := range parts {
		consumedRunes += runeCounts[index]
		endMs := durationMs
		if durationMs > 0 && totalRunes > 0 && index < len(parts)-1 {
			endMs = int(math.Round(float64(consumedRunes) / float64(totalRunes) * float64(durationMs)))
		}
		segments = append(segments, gin.H{
			"start_ms": startMs,
			"end_ms":   endMs,
			"text":     strings.TrimSpace(part),
		})
		startMs = endMs
	}
	return segments
}

func splitOpenTranscriptSegments(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	segments := make([]string, 0, 4)
	var builder strings.Builder
	flush := func() {
		segment := strings.TrimSpace(builder.String())
		if segment != "" {
			segments = append(segments, segment)
		}
		builder.Reset()
	}
	for _, item := range []rune(trimmed) {
		builder.WriteRune(item)
		if isOpenTranscriptBoundary(item) {
			flush()
		}
	}
	flush()
	if len(segments) == 0 {
		return []string{trimmed}
	}
	return segments
}

func isOpenTranscriptBoundary(item rune) bool {
	switch item {
	case '\n', '。', '！', '？', '；', '…', '.', '!', '?', ';':
		return true
	default:
		return false
	}
}

func openStreamTextDelta(previous, current string) string {
	trimmedPrevious := strings.TrimSpace(previous)
	trimmedCurrent := strings.TrimSpace(current)
	if trimmedCurrent == "" || trimmedCurrent == trimmedPrevious {
		return ""
	}
	if trimmedPrevious == "" {
		return trimmedCurrent
	}
	if strings.HasPrefix(trimmedCurrent, trimmedPrevious) {
		return strings.TrimSpace(trimmedCurrent[len(trimmedPrevious):])
	}
	return trimmedCurrent
}

func buildTranscriptText(items []appmeeting.TranscriptItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n")
}

func summaryContent(result *appmeeting.MeetingDetailResponse) string {
	if result == nil || result.Summary == nil {
		return ""
	}
	return strings.TrimSpace(result.Summary.Content)
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func queryOrBodyWorkflowID(c *gin.Context) string {
	if value := strings.TrimSpace(c.Query("workflow_id")); value != "" {
		return value
	}
	var body struct {
		WorkflowID *uint64 `json:"workflow_id"`
	}
	if err := c.ShouldBindJSON(&body); err == nil && body.WorkflowID != nil {
		return strconv.FormatUint(*body.WorkflowID, 10)
	}
	return ""
}

func filepathExt(name string) string {
	index := strings.LastIndex(name, ".")
	if index < 0 || index == len(name)-1 {
		return ""
	}
	return name[index+1:]
}
