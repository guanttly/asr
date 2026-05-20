package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	appasr "github.com/lgt/asr/internal/application/asr"
	appaudio "github.com/lgt/asr/internal/application/audio"
	appnlp "github.com/lgt/asr/internal/application/nlp"
	asrdomain "github.com/lgt/asr/internal/domain/asr"
)

const legacyOwnerID uint64 = 0

type legacyEnvelope struct {
	Success        bool   `json:"success"`
	Message        string `json:"message,omitempty"`
	Data           any    `json:"data,omitempty"`
	PartialSuccess bool   `json:"partial_success,omitempty"`
	ErrorDetails   string `json:"error_details,omitempty"`
}

func legacySuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, legacyEnvelope{Success: true, Data: data})
}

func legacyMessage(c *gin.Context, message string, data any) {
	c.JSON(http.StatusOK, legacyEnvelope{Success: true, Message: message, Data: data})
}

func legacyError(c *gin.Context, status int, message string) {
	c.JSON(status, legacyEnvelope{Success: false, Message: message})
}

type LegacyASRHandler struct {
	asrService     *appasr.Service
	audioService   *appaudio.Service
	nlpService     *appnlp.Service
	uploadDir      string
	publicBaseURL  string
	maxAudioSizeMB int64
	httpClient     *http.Client
}

func NewLegacyASRHandler(asrService *appasr.Service, nlpService *appnlp.Service, uploadDir, publicBaseURL string, maxAudioSizeMB int64) *LegacyASRHandler {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "uploads"
	}
	if maxAudioSizeMB <= 0 {
		maxAudioSizeMB = 1024
	}
	return &LegacyASRHandler{
		asrService:     asrService,
		audioService:   appaudio.NewService(asrService, nil),
		nlpService:     nlpService,
		uploadDir:      uploadDir,
		publicBaseURL:  strings.TrimRight(publicBaseURL, "/"),
		maxAudioSizeMB: maxAudioSizeMB,
		httpClient:     &http.Client{Timeout: 10 * time.Second},
	}
}

type legacySummaryOptions struct {
	Filename         string
	FileSize         int64
	TemplateName     string
	EnableCorrection bool
	EnableSpeaker    bool
	StartedAt        time.Time
}

func legacyBoolForm(c *gin.Context, name string, defaultValue bool) bool {
	switch strings.ToLower(strings.TrimSpace(c.PostForm(name))) {
	case "true", "1", "yes", "y", "on":
		return true
	case "false", "0", "no", "n", "off":
		return false
	default:
		return defaultValue
	}
}

func legacyLanguageForm(c *gin.Context) string {
	language := strings.TrimSpace(c.PostForm("language"))
	if language == "" {
		return appasr.DefaultLanguage
	}
	return language
}

func legacyHotwordsForm(c *gin.Context) []string {
	raw := strings.TrimSpace(c.PostForm("hotwords"))
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r'
	})
	hotwords := make([]string, 0, len(parts))
	for _, part := range parts {
		word := strings.TrimSpace(part)
		if word != "" {
			hotwords = append(hotwords, word)
		}
	}
	return hotwords
}

func legacyCallbackURLForm(c *gin.Context) string {
	if callbackURL := strings.TrimSpace(c.PostForm("callback")); callbackURL != "" {
		return callbackURL
	}
	return strings.TrimSpace(c.PostForm("callback_url"))
}

func isValidLegacyCallbackURL(callbackURL string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(callbackURL))
	if err != nil || parsed == nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && strings.TrimSpace(parsed.Host) != ""
}

func (h *LegacyASRHandler) Register(group *gin.RouterGroup) {
	group.POST("/upload", h.Upload)
	group.POST("/recognize", h.Recognize)
	group.POST("/recognize/vad", h.RecognizeVAD)
	group.POST("/audio/to_summary", h.AudioToSummary)
	group.GET("/task/:task_id", h.GetTask)
}

func (h *LegacyASRHandler) Upload(c *gin.Context) {
	useVAD := legacyBoolForm(c, "use_vad_segmentation", false)
	h.recognize(c, useVAD)
}

func (h *LegacyASRHandler) Recognize(c *gin.Context) {
	h.recognize(c, false)
}

func (h *LegacyASRHandler) RecognizeVAD(c *gin.Context) {
	h.recognize(c, true)
}

func (h *LegacyASRHandler) recognize(c *gin.Context, useVAD bool) {
	language := legacyLanguageForm(c)
	useITN := legacyBoolForm(c, "use_itn", true)
	hotwords := legacyHotwordsForm(c)

	audioFile, err := saveTemporaryUploadedAudio(c, "file", "legacy-asr", h.maxAudioSizeMB)
	if err != nil {
		status, messageText := resolveAudioUploadError(err)
		legacyError(c, status, messageText)
		return
	}
	defer func() { _ = os.Remove(audioFile.AbsolutePath) }()

	result, err := h.asrService.TranscribeSnippet(c.Request.Context(), &appasr.TranscribeSnippetRequest{
		LocalFilePath: audioFile.AbsolutePath,
		Language:      language,
		UseITN:        &useITN,
		Hotwords:      hotwords,
	})
	if err != nil {
		legacyError(c, http.StatusInternalServerError, err.Error())
		return
	}
	payload := gin.H{
		"text":     result.Text,
		"duration": result.Duration,
	}
	if useVAD {
		payload["segments"] = []gin.H{{
			"start_ms": 0,
			"end_ms":   int(result.Duration * 1000),
			"text":     result.Text,
		}}
	}
	c.JSON(http.StatusOK, gin.H{
		"success":              true,
		"filename":             audioFile.OriginalFilename,
		"language":             language,
		"use_vad_segmentation": useVAD,
		"result":               payload,
		"data":                 payload,
		"timestamp":            time.Now().Format(time.RFC3339Nano),
	})
}

func (h *LegacyASRHandler) AudioToSummary(c *gin.Context) {
	startedAt := time.Now()
	templateName := strings.TrimSpace(c.PostForm("template_name"))
	if templateName == "" {
		templateName = "default"
	}
	enableCorrection := legacyBoolForm(c, "enable_correction", true)
	enableSpeaker := legacyBoolForm(c, "enable_speaker", false)
	language := legacyLanguageForm(c)
	useITN := legacyBoolForm(c, "use_itn", true)
	callbackURL := legacyCallbackURLForm(c)
	if callbackURL != "" && !isValidLegacyCallbackURL(callbackURL) {
		legacyError(c, http.StatusBadRequest, "callback URL格式不正确，请提供有效的HTTP/HTTPS URL")
		return
	}

	audioFile, err := savePermanentUploadedAudio(c, "audio_file", h.uploadDir, "audio", h.maxAudioSizeMB)
	if err != nil {
		status, messageText := resolveAudioUploadError(err)
		legacyError(c, status, messageText)
		return
	}
	audioURL, err := buildUploadedFileURL(c, h.publicBaseURL, audioFile.RelativePath)
	if err != nil {
		legacyError(c, http.StatusInternalServerError, err.Error())
		return
	}
	task, err := h.audioService.CreateBatchTaskFromAudio(c.Request.Context(), legacyOwnerID, appaudio.CreateBatchTaskRequest{
		Audio: appaudio.PreparedAudio{
			OriginalFilename: audioFile.OriginalFilename,
			AudioURL:         audioURL,
			LocalFilePath:    audioFile.AbsolutePath,
			Duration:         audioFile.Duration,
		},
		Language: language,
		UseITN:   &useITN,
	})
	if err != nil {
		legacyError(c, http.StatusInternalServerError, err.Error())
		return
	}
	options := legacySummaryOptions{
		Filename:         audioFile.OriginalFilename,
		FileSize:         audioFile.Size,
		TemplateName:     templateName,
		EnableCorrection: enableCorrection,
		EnableSpeaker:    enableSpeaker,
		StartedAt:        startedAt,
	}
	if callbackURL != "" {
		go h.dispatchSummaryCallback(task.ID, callbackURL, options)
		legacyMessage(c, "任务已创建，将通过回调返回结果", gin.H{
			"task_id":      openTaskID(task.ID),
			"callback_url": callbackURL,
			"created_at":   task.CreatedAt,
		})
		return
	}
	result, err := h.waitLegacyTask(c.Request.Context(), task.ID, 5*time.Minute)
	if err != nil {
		legacyError(c, http.StatusGatewayTimeout, err.Error())
		return
	}
	legacySuccess(c, h.buildLegacySummaryPayload(c.Request.Context(), options, result))
}

func (h *LegacyASRHandler) GetTask(c *gin.Context) {
	taskID, err := parseOpenTaskID(c.Param("task_id"))
	if err != nil {
		legacyError(c, http.StatusBadRequest, "invalid task_id")
		return
	}
	result, err := h.asrService.GetTask(c.Request.Context(), legacyOwnerID, taskID)
	if err != nil {
		legacyError(c, http.StatusNotFound, "任务不存在或已完成")
		return
	}
	legacySuccess(c, gin.H{
		"task_id":     openTaskID(result.ID),
		"status":      toOpenTaskStatus(result.Status),
		"progress":    float64(result.ProgressPercent) / 100,
		"result_text": result.ResultText,
		"duration":    result.Duration,
	})
}

func (h *LegacyASRHandler) waitLegacyTask(ctx context.Context, taskID uint64, timeout time.Duration) (*appasr.TaskResponse, error) {
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		result, err := h.asrService.GetTask(pollCtx, legacyOwnerID, taskID)
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

func (h *LegacyASRHandler) buildLegacySummaryPayload(ctx context.Context, options legacySummaryOptions, result *appasr.TaskResponse) gin.H {
	content := result.ResultText
	modelVersion := "summary"
	if h.nlpService != nil && strings.TrimSpace(result.ResultText) != "" {
		summary, err := h.nlpService.Summarize(ctx, &appnlp.SummarizeRequest{Text: result.ResultText})
		if err == nil {
			content = summary.Content
			modelVersion = summary.ModelVersion
		}
	}
	return gin.H{
		"asr_result": gin.H{
			"text":     result.ResultText,
			"duration": result.Duration,
		},
		"llm_processing": gin.H{
			"summary": gin.H{
				"content":       content,
				"model_version": modelVersion,
			},
		},
		"processing_info": gin.H{
			"filename":                       options.Filename,
			"file_size":                      options.FileSize,
			"template_used":                  options.TemplateName,
			"correction_enabled":             options.EnableCorrection,
			"speaker_identification_enabled": options.EnableSpeaker,
			"processing_time": gin.H{
				"total_duration": legacyElapsedSeconds(options.StartedAt),
			},
		},
	}
}

func legacyElapsedSeconds(startedAt time.Time) float64 {
	if startedAt.IsZero() {
		return 0
	}
	return float64(int(time.Since(startedAt).Seconds()*10)) / 10
}

func (h *LegacyASRHandler) dispatchSummaryCallback(taskID uint64, callbackURL string, options legacySummaryOptions) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := h.waitLegacyTask(ctx, taskID, 10*time.Minute)
	if err != nil {
		h.postSummaryCallback(callbackURL, legacyEnvelope{Success: false, Message: fmt.Sprintf("处理失败: %s", err.Error())})
		return
	}
	payload := h.buildLegacySummaryPayload(context.Background(), options, result)
	h.postSummaryCallback(callbackURL, legacyEnvelope{Success: true, Data: payload})
}

func (h *LegacyASRHandler) postSummaryCallback(callbackURL string, envelope legacyEnvelope) {
	body, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	for _, backoff := range []time.Duration{0, time.Second, 5 * time.Second} {
		if backoff > 0 {
			time.Sleep(backoff)
		}
		request, reqErr := http.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(body))
		if reqErr != nil {
			return
		}
		request.Header.Set("Content-Type", "application/json")
		response, respErr := h.httpClient.Do(request)
		if respErr == nil && response != nil {
			_ = response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				return
			}
		}
	}
}

type LegacyNLPHandler struct {
	service *appnlp.Service
}

func NewLegacyNLPHandler(service *appnlp.Service) *LegacyNLPHandler {
	return &LegacyNLPHandler{service: service}
}

func (h *LegacyNLPHandler) Register(group *gin.RouterGroup) {
	group.POST("/meeting/summary", h.MeetingSummary)
	group.POST("/text/correct", h.Correct)
	group.GET("/templates", h.Templates)
}

func (h *LegacyNLPHandler) MeetingSummary(c *gin.Context) {
	var req appnlp.SummarizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		legacyError(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.service.Summarize(c.Request.Context(), &req)
	if err != nil {
		legacyError(c, http.StatusInternalServerError, err.Error())
		return
	}
	legacySuccess(c, result)
}

func (h *LegacyNLPHandler) Correct(c *gin.Context) {
	var req appnlp.CorrectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		legacyError(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.service.Correct(c.Request.Context(), &req)
	if err != nil {
		legacyError(c, http.StatusInternalServerError, err.Error())
		return
	}
	legacySuccess(c, result)
}

func (h *LegacyNLPHandler) Templates(c *gin.Context) {
	legacySuccess(c, gin.H{
		"templates":        []string{"default"},
		"default_template": "default",
	})
}
