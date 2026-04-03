package diarization

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
		return nil, fmt.Errorf("diarization service returned status %d", resp.StatusCode)
	}

	var segments []Segment
	if err := json.NewDecoder(resp.Body).Decode(&segments); err != nil {
		return nil, err
	}

	return segments, nil
}
