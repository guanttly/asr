package diarization

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Client wraps calls to an external diarization service.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// Segment describes a diarization result.
type Segment struct {
	Speaker   string  `json:"speaker"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// NewClient creates a new diarization client.
func NewClient(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseURL:    baseURL,
	}
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

	var segments []Segment
	if err := json.NewDecoder(resp.Body).Decode(&segments); err != nil {
		return nil, err
	}

	return segments, nil
}

// AnalyzeFile uploads a local audio file to the diarization service.
func (c *Client) AnalyzeFile(ctx context.Context, audioFilePath string) ([]Segment, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/diarize", c.baseURL), &body)
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

	var segments []Segment
	if err := json.NewDecoder(resp.Body).Decode(&segments); err != nil {
		return nil, err
	}

	return segments, nil
}

func buildDiarizationServiceError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := bytes.TrimSpace(body)
	if len(message) == 0 {
		return fmt.Errorf("diarization service returned status %d", resp.StatusCode)
	}
	return fmt.Errorf("diarization service returned status %d: %s", resp.StatusCode, string(message))
}
