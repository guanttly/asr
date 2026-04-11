package asrengine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/lgt/asr/internal/infrastructure/audiofile"
)

func (c *Client) submitSplitOpenAITranscription(ctx context.Context, req BatchTranscribeRequest) (*BatchTranscribeResponse, error) {
	segmentPaths, cleanup, err := c.splitLocalFileForTranscription(ctx, req.LocalFilePath)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	if req.Progress != nil {
		req.Progress(BatchTranscribeProgress{SegmentTotal: len(segmentPaths), SegmentCompleted: 0})
	}

	texts := make([]string, 0, len(segmentPaths))
	var totalDuration float64
	for index, segmentPath := range segmentPaths {
		result, err := c.submitOpenAITranscriptionSingle(ctx, BatchTranscribeRequest{
			LocalFilePath: segmentPath,
			AudioURL:      req.AudioURL,
			DictID:        req.DictID,
		})
		if err != nil {
			return nil, err
		}

		if text := strings.TrimSpace(result.ResultText); text != "" {
			texts = append(texts, text)
		}
		totalDuration += result.Duration
		if req.Progress != nil {
			req.Progress(BatchTranscribeProgress{SegmentTotal: len(segmentPaths), SegmentCompleted: index + 1})
		}
	}

	return &BatchTranscribeResponse{
		Status:     "completed",
		ResultText: strings.Join(texts, "\n"),
		Duration:   totalDuration,
	}, nil
}

func (c *Client) submitOpenAITranscriptionSingle(ctx context.Context, req BatchTranscribeRequest) (*BatchTranscribeResponse, error) {
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

func (c *Client) splitLocalFileForTranscription(ctx context.Context, localPath string) ([]string, func(), error) {
	if strings.TrimSpace(localPath) == "" {
		return nil, nil, fmt.Errorf("local audio file path is required for auto split transcription")
	}
	if c.maxAudioSizeBytes() <= 0 {
		return nil, nil, fmt.Errorf("services.asr_max_audio_size_mb must be greater than 0 to enable auto split transcription")
	}

	workingDir, err := os.MkdirTemp(filepath.Dir(localPath), "asr-split-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create split workspace: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(workingDir)
	}

	segmentPaths, _, err := audiofile.SplitForMaxBytes(ctx, localPath, workingDir, c.maxAudioSizeBytes())
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return segmentPaths, cleanup, nil
}
