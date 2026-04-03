package asrengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
)

type batchSubmitMode uint8

const (
	batchSubmitModeUnknown batchSubmitMode = iota
	batchSubmitModeTaskAPI
	batchSubmitModeOpenAICompatible
)

// Client wraps calls to the external ASR engine service.
type Client struct {
	httpClient     *http.Client
	baseURL        string
	streamURL      string // base URL for the qwen-asr-demo-streaming HTTP API
	maxAudioSizeMB int64

	batchSubmitModeMu sync.RWMutex
	batchSubmitMode   batchSubmitMode
}

// BatchTranscribeRequest is the request body for batch transcription.
type BatchTranscribeRequest struct {
	AudioURL      string                        `json:"audio_url"`
	LocalFilePath string                        `json:"-"`
	DictID        *uint64                       `json:"dict_id,omitempty"`
	Progress      func(BatchTranscribeProgress) `json:"-"`
}

// BatchTranscribeProgress describes segment-based progress for locally split uploads.
type BatchTranscribeProgress struct {
	SegmentTotal     int
	SegmentCompleted int
}

// BatchTranscribeResponse is the response returned by the engine.
type BatchTranscribeResponse struct {
	TaskID     string  `json:"task_id"`
	Status     string  `json:"status,omitempty"`
	ResultText string  `json:"result_text,omitempty"`
	Duration   float64 `json:"duration,omitempty"`
}

type openAITranscriptionResponse struct {
	Text  string `json:"text"`
	Usage struct {
		Seconds float64 `json:"seconds"`
	} `json:"usage"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Param   string `json:"param"`
		Code    int    `json:"code"`
	} `json:"error"`
}

type UpstreamBadRequestError struct {
	Message string
}

func (e *UpstreamBadRequestError) Error() string {
	return e.Message
}

// BatchTaskQueryResponse is the normalized response returned by the engine query API.
type BatchTaskQueryResponse struct {
	Status     string
	ResultText string
	Duration   float64
}

// NewClient creates a reusable ASR engine client.
func NewClient(baseURL, streamURL string, maxAudioSizeMB int64) *Client {
	return &Client{
		httpClient:     &http.Client{},
		baseURL:        baseURL,
		streamURL:      streamURL,
		maxAudioSizeMB: maxAudioSizeMB,
	}
}

// StreamURL returns the configured streaming endpoint.
func (c *Client) StreamURL() string {
	return c.streamURL
}

// StreamingSessionResult holds the response from the streaming HTTP API.
type StreamingSessionResult struct {
	SessionID string `json:"session_id,omitempty"`
	Language  string `json:"language"`
	Text      string `json:"text"`
}

// StartStreamSession creates a new streaming session on the upstream ASR engine.
func (c *Client) StartStreamSession(ctx context.Context) (string, error) {
	if strings.TrimSpace(c.streamURL) == "" {
		return "", fmt.Errorf("services.asr_stream is not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.streamURL, "/")+"/api/start", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("start stream session: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result StreamingSessionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("start stream session: decode response: %w", err)
	}

	return result.SessionID, nil
}

// PushStreamChunk sends a float32 PCM audio chunk to an active streaming session.
func (c *Client) PushStreamChunk(ctx context.Context, sessionID string, pcmData []byte) (*StreamingSessionResult, error) {
	if strings.TrimSpace(c.streamURL) == "" {
		return nil, fmt.Errorf("services.asr_stream is not configured")
	}

	reqURL := fmt.Sprintf("%s/api/chunk?session_id=%s", strings.TrimRight(c.streamURL, "/"), url.QueryEscape(sessionID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(pcmData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("push stream chunk: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result StreamingSessionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("push stream chunk: decode response: %w", err)
	}

	return &result, nil
}

// FinishStreamSession finalizes a streaming session and returns the final result.
func (c *Client) FinishStreamSession(ctx context.Context, sessionID string) (*StreamingSessionResult, error) {
	if strings.TrimSpace(c.streamURL) == "" {
		return nil, fmt.Errorf("services.asr_stream is not configured")
	}

	reqURL := fmt.Sprintf("%s/api/finish?session_id=%s", strings.TrimRight(c.streamURL, "/"), url.QueryEscape(sessionID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("finish stream session: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result StreamingSessionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("finish stream session: decode response: %w", err)
	}

	return &result, nil
}

// SubmitBatch submits a batch transcription task to the external engine.
func (c *Client) SubmitBatch(ctx context.Context, req BatchTranscribeRequest) (*BatchTranscribeResponse, error) {
	if strings.TrimSpace(c.baseURL) == "" {
		return nil, fmt.Errorf("services.asr is not configured")
	}

	if c.getBatchSubmitMode() == batchSubmitModeOpenAICompatible {
		return c.submitOpenAITranscription(ctx, req)
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/transcribe", c.baseURL), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			c.setBatchSubmitMode(batchSubmitModeOpenAICompatible)
			fallback, fallbackErr := c.submitOpenAITranscription(ctx, req)
			if fallbackErr == nil {
				return fallback, nil
			}
			var badRequestErr *UpstreamBadRequestError
			if errors.As(fallbackErr, &badRequestErr) {
				return nil, badRequestErr
			}
			return nil, fmt.Errorf("configured ASR endpoint %s does not implement POST /transcribe, and OpenAI-compatible POST /v1/audio/transcriptions also failed: %v. upstream body: %s", strings.TrimRight(c.baseURL, "/"), fallbackErr, strings.TrimSpace(string(body)))
		}
		return nil, fmt.Errorf("batch asr request to %s/transcribe returned status %d: %s", strings.TrimRight(c.baseURL, "/"), resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result BatchTranscribeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	c.setBatchSubmitMode(batchSubmitModeTaskAPI)

	return &result, nil
}

func (c *Client) getBatchSubmitMode() batchSubmitMode {
	c.batchSubmitModeMu.RLock()
	defer c.batchSubmitModeMu.RUnlock()
	return c.batchSubmitMode
}

func (c *Client) setBatchSubmitMode(mode batchSubmitMode) {
	c.batchSubmitModeMu.Lock()
	defer c.batchSubmitModeMu.Unlock()
	c.batchSubmitMode = mode
}

func (c *Client) submitOpenAITranscription(ctx context.Context, req BatchTranscribeRequest) (*BatchTranscribeResponse, error) {
	if c.shouldSplitLocalFile(req.LocalFilePath) {
		return c.submitSplitOpenAITranscription(ctx, req)
	}

	sourceBody, filename, cleanup, err := c.openTranscriptionSource(ctx, req)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	bodyReader, bodyWriter := io.Pipe()
	formWriter := multipart.NewWriter(bodyWriter)

	go func() {
		defer bodyWriter.Close()

		fileWriter, createErr := formWriter.CreateFormFile("file", filename)
		if createErr != nil {
			_ = bodyWriter.CloseWithError(createErr)
			return
		}

		if _, copyErr := io.Copy(fileWriter, sourceBody); copyErr != nil {
			_ = bodyWriter.CloseWithError(copyErr)
			return
		}

		if closeErr := formWriter.Close(); closeErr != nil {
			_ = bodyWriter.CloseWithError(closeErr)
		}
	}()

	endpoint := strings.TrimRight(c.baseURL, "/") + "/v1/audio/transcriptions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bodyReader)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", formWriter.FormDataContentType())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, newOpenAIBatchRequestError(endpoint, resp.StatusCode, body)
	}

	var result openAITranscriptionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode OpenAI-compatible batch asr response: %w", err)
	}

	return &BatchTranscribeResponse{
		Status:     "completed",
		ResultText: result.Text,
		Duration:   result.Usage.Seconds,
	}, nil
}

func newOpenAIBatchRequestError(endpoint string, statusCode int, body []byte) error {
	bodyText := strings.TrimSpace(string(body))
	if statusCode != http.StatusBadRequest {
		return fmt.Errorf("OpenAI-compatible batch asr request to %s returned status %d: %s", endpoint, statusCode, bodyText)
	}

	var payload openAIErrorResponse
	if err := json.Unmarshal(body, &payload); err == nil {
		message := strings.TrimSpace(payload.Error.Message)
		if payload.Error.Param == "audio_filesize_mb" && message != "" {
			return &UpstreamBadRequestError{Message: fmt.Sprintf("音频文件超过 ASR 上游限制。上游原始信息：%s。请先切分音频，或提高上游服务的 audio_filesize_mb 配置", message)}
		}
		if message != "" {
			return &UpstreamBadRequestError{Message: message}
		}
	}

	return &UpstreamBadRequestError{Message: fmt.Sprintf("OpenAI-compatible batch asr request to %s returned status %d: %s", endpoint, statusCode, bodyText)}
}

func (c *Client) shouldSplitLocalFile(localPath string) bool {
	if strings.TrimSpace(localPath) == "" || c.maxAudioSizeMB <= 0 {
		return false
	}

	info, err := os.Stat(localPath)
	if err != nil {
		return false
	}

	return info.Size() > c.maxAudioSizeBytes()
}

func (c *Client) maxAudioSizeBytes() int64 {
	if c.maxAudioSizeMB <= 0 {
		return 0
	}
	return c.maxAudioSizeMB * 1024 * 1024
}

func (c *Client) openTranscriptionSource(ctx context.Context, req BatchTranscribeRequest) (io.ReadCloser, string, func(), error) {
	if localPath := strings.TrimSpace(req.LocalFilePath); localPath != "" {
		file, err := os.Open(localPath)
		if err != nil {
			return nil, "", nil, fmt.Errorf("open local audio file %s: %w", localPath, err)
		}

		return file, path.Base(localPath), func() {
			_ = file.Close()
		}, nil
	}

	if strings.TrimSpace(req.AudioURL) == "" {
		return nil, "", nil, fmt.Errorf("audio_url or local uploaded file is required for OpenAI-compatible ASR submission")
	}

	sourceReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.AudioURL, nil)
	if err != nil {
		return nil, "", nil, fmt.Errorf("prepare audio fetch request: %w", err)
	}

	sourceResp, err := c.httpClient.Do(sourceReq)
	if err != nil {
		return nil, "", nil, fmt.Errorf("fetch audio source %s: %w", req.AudioURL, err)
	}
	if sourceResp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(sourceResp.Body)
		sourceResp.Body.Close()
		return nil, "", nil, fmt.Errorf("fetch audio source %s returned status %d: %s", req.AudioURL, sourceResp.StatusCode, strings.TrimSpace(string(body)))
	}

	filename := transcriptionFilename(req.AudioURL)
	return sourceResp.Body, filename, func() {
		_ = sourceResp.Body.Close()
	}, nil
}

func transcriptionFilename(audioURL string) string {
	parsed, err := url.Parse(audioURL)
	if err == nil {
		name := path.Base(parsed.Path)
		if name != "" && name != "." && name != "/" {
			return name
		}
	}
	return "audio.wav"
}

// QueryBatchTask retrieves a batch transcription task state from the external engine.
func (c *Client) QueryBatchTask(ctx context.Context, taskID string) (*BatchTaskQueryResponse, error) {
	if strings.TrimSpace(c.baseURL) == "" {
		return nil, fmt.Errorf("services.asr is not configured")
	}

	paths := []string{
		fmt.Sprintf("%s/transcribe/%s", c.baseURL, taskID),
		fmt.Sprintf("%s/tasks/%s", c.baseURL, taskID),
	}

	var lastErr error
	for _, path := range paths {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}

		if resp.StatusCode == http.StatusNotFound {
			lastErr = fmt.Errorf("configured batch ASR endpoint %s does not implement batch task query for task %s (status 404)", strings.TrimRight(c.baseURL, "/"), taskID)
			continue
		}
		if resp.StatusCode >= http.StatusBadRequest {
			return nil, fmt.Errorf("batch asr task query %s returned status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
		}

		return parseBatchTaskQuery(body)
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("asr engine task query failed")
}

func parseBatchTaskQuery(payload []byte) (*BatchTaskQueryResponse, error) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}

	data := unwrapData(raw)
	status := firstString(data, "status", "state", "task_status")
	resultText := firstString(data, "result_text", "text", "transcript", "content", "result")
	if nested, ok := data["result"].(map[string]any); ok {
		if resultText == "" {
			resultText = firstString(nested, "text", "transcript", "content", "result_text")
		}
	}

	return &BatchTaskQueryResponse{
		Status:     status,
		ResultText: resultText,
		Duration:   firstFloat(data, "duration", "audio_duration", "length"),
	}, nil
}

func unwrapData(raw map[string]any) map[string]any {
	if data, ok := raw["data"].(map[string]any); ok {
		return data
	}
	return raw
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return typed
		case []byte:
			return string(typed)
		}
	}
	return ""
}

func firstFloat(raw map[string]any, keys ...string) float64 {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return typed
		case float32:
			return float64(typed)
		case int:
			return float64(typed)
		case int64:
			return float64(typed)
		case json.Number:
			parsed, err := typed.Float64()
			if err == nil {
				return parsed
			}
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
			if err == nil {
				return parsed
			}
		}
	}
	return 0
}
