package diarization

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
	"path/filepath"
	"strings"
)

type AnalyzeOptions struct {
	EnableVoiceprintMatch bool
}

type VoiceprintMetadata struct {
	SpeakerName string
	Department  string
	Notes       string
}

type VoiceprintRecord struct {
	ID            string  `json:"id"`
	SpeakerName   string  `json:"speaker_name"`
	Department    string  `json:"department"`
	Notes         string  `json:"notes"`
	EmbeddingPath string  `json:"embedding_path"`
	AudioPath     string  `json:"audio_path"`
	AudioDuration float64 `json:"audio_duration"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

// Client wraps calls to an external diarization service.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

type UpstreamServiceError struct {
	statusCode int
	message    string
}

func (e *UpstreamServiceError) Error() string {
	if strings.TrimSpace(e.message) == "" {
		return fmt.Sprintf("diarization service returned status %d", e.statusCode)
	}
	return fmt.Sprintf("diarization service returned status %d: %s", e.statusCode, e.message)
}

func (e *UpstreamServiceError) HTTPStatusCode() int {
	return e.statusCode
}

// Segment describes a diarization result.
type Segment struct {
	Speaker   string  `json:"speaker"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

func (s *Segment) UnmarshalJSON(data []byte) error {
	var raw struct {
		Speaker   string   `json:"speaker"`
		SpeakerID string   `json:"speaker_id"`
		StartTime *float64 `json:"start_time"`
		EndTime   *float64 `json:"end_time"`
		Start     *float64 `json:"start"`
		End       *float64 `json:"end"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	s.Speaker = strings.TrimSpace(raw.Speaker)
	if s.Speaker == "" {
		s.Speaker = strings.TrimSpace(raw.SpeakerID)
	}
	if raw.StartTime != nil {
		s.StartTime = *raw.StartTime
	} else if raw.Start != nil {
		s.StartTime = *raw.Start
	}
	if raw.EndTime != nil {
		s.EndTime = *raw.EndTime
	} else if raw.End != nil {
		s.EndTime = *raw.End
	}
	return nil
}

// NewClient creates a new diarization client.
func NewClient(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{},
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

func (c *Client) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.baseURL
}

// Analyze requests diarization for an audio URL.
func (c *Client) Analyze(ctx context.Context, audioURL string) ([]Segment, error) {
	payload, err := json.Marshal(map[string]string{"audio_url": audioURL})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/diarize", c.baseURL), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, buildDiarizationServiceError(resp)
	}

	segments, err := decodeSegmentsResponse(resp.Body)
	if err != nil {
		return nil, err
	}

	return segments, nil
}

// AnalyzeFile uploads a local audio file to the diarization service.
func (c *Client) AnalyzeFile(ctx context.Context, audioFilePath string) ([]Segment, error) {
	return c.analyzeFile(ctx, audioFilePath, AnalyzeOptions{})
}

func (c *Client) AnalyzeFileWithOptions(ctx context.Context, audioFilePath string, options AnalyzeOptions) ([]Segment, error) {
	return c.analyzeFile(ctx, audioFilePath, options)
}

func (c *Client) ListVoiceprints(ctx context.Context) ([]VoiceprintRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiV1Endpoint("/voiceprint/list"), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, buildDiarizationServiceError(resp)
	}

	return decodeVoiceprintListResponse(resp.Body)
}

func (c *Client) EnrollVoiceprint(ctx context.Context, audioFilePath string, metadata VoiceprintMetadata) (*VoiceprintRecord, error) {
	file, err := os.Open(audioFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(audioFilePath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}
	if err := writer.WriteField("speaker_name", metadata.SpeakerName); err != nil {
		return nil, err
	}
	if strings.TrimSpace(metadata.Department) != "" {
		if err := writer.WriteField("department", metadata.Department); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(metadata.Notes) != "" {
		if err := writer.WriteField("notes", metadata.Notes); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiV1Endpoint("/voiceprint/enroll"), &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, buildDiarizationServiceError(resp)
	}

	return decodeVoiceprintRecordResponse(resp.Body)
}

func (c *Client) DeleteVoiceprint(ctx context.Context, recordID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.apiV1Endpoint("/voiceprint/"+url.PathEscape(recordID)), nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return buildDiarizationServiceError(resp)
	}

	return nil
}

func (c *Client) analyzeFile(ctx context.Context, audioFilePath string, options AnalyzeOptions) ([]Segment, error) {
	file, err := os.Open(audioFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(audioFilePath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.fileAnalyzeEndpoint(options), &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, buildDiarizationServiceError(resp)
	}

	segments, err := decodeSegmentsResponse(resp.Body)
	if err != nil {
		return nil, err
	}

	return segments, nil
}

func (c *Client) fileAnalyzeEndpoint(options AnalyzeOptions) string {
	baseURL := strings.TrimRight(c.baseURL, "/")
	if options.EnableVoiceprintMatch {
		return c.apiV1Endpoint("/diarize-identify")
	}
	if strings.Contains(baseURL, "/api/v1") {
		return baseURL + "/diarize"
	}
	return baseURL + "/diarize"
}

func (c *Client) apiV1Endpoint(path string) string {
	baseURL := strings.TrimRight(c.baseURL, "/")
	normalizedPath := path
	if !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}
	if strings.Contains(baseURL, "/api/v1") {
		return baseURL + normalizedPath
	}
	return baseURL + "/api/v1" + normalizedPath
}

func decodeSegmentsResponse(body io.Reader) ([]Segment, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty diarization response")
	}

	var segments []Segment
	if err := json.Unmarshal(trimmed, &segments); err == nil {
		return segments, nil
	} else {
		var wrapped struct {
			Segments []Segment `json:"segments"`
		}
		if wrappedErr := json.Unmarshal(trimmed, &wrapped); wrappedErr == nil && wrapped.Segments != nil {
			return wrapped.Segments, nil
		}
		return nil, err
	}
}

func decodeVoiceprintListResponse(body io.Reader) ([]VoiceprintRecord, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty voiceprint response")
	}

	var wrapped struct {
		Records []VoiceprintRecord `json:"records"`
	}
	if err := json.Unmarshal(trimmed, &wrapped); err == nil && wrapped.Records != nil {
		return wrapped.Records, nil
	}

	var records []VoiceprintRecord
	if err := json.Unmarshal(trimmed, &records); err == nil {
		return records, nil
	}

	return nil, fmt.Errorf("invalid voiceprint list response")
}

func decodeVoiceprintRecordResponse(body io.Reader) (*VoiceprintRecord, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty voiceprint response")
	}

	var record VoiceprintRecord
	if err := json.Unmarshal(trimmed, &record); err == nil && strings.TrimSpace(record.ID) != "" {
		return &record, nil
	}

	var wrapped struct {
		Record VoiceprintRecord `json:"record"`
	}
	if err := json.Unmarshal(trimmed, &wrapped); err == nil && strings.TrimSpace(wrapped.Record.ID) != "" {
		return &wrapped.Record, nil
	}

	return nil, fmt.Errorf("invalid voiceprint record response")
}

func buildDiarizationServiceError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := bytes.TrimSpace(body)
	return &UpstreamServiceError{statusCode: resp.StatusCode, message: string(message)}
}

func IsUpstreamServiceError(err error) bool {
	var serviceErr *UpstreamServiceError
	return errors.As(err, &serviceErr)
}
