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
	"strings"
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

	segments, err := decodeSegmentsResponse(resp.Body)
	if err != nil {
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

	segments, err := decodeSegmentsResponse(resp.Body)
	if err != nil {
		return nil, err
	}

	return segments, nil
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

func buildDiarizationServiceError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := bytes.TrimSpace(body)
	if len(message) == 0 {
		return fmt.Errorf("diarization service returned status %d", resp.StatusCode)
	}
	return fmt.Errorf("diarization service returned status %d: %s", resp.StatusCode, string(message))
}
