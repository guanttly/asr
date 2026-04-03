package asrengine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	splitSafetyRatio        = 0.85
	minSplitDurationSeconds = 15.0
	maxSplitAttempts        = 5
)

type ffprobeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

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
	if err := ensureCommandAvailable("ffmpeg"); err != nil {
		return nil, nil, fmt.Errorf("audio file exceeds ASR upstream limit and requires auto split, but ffmpeg is not available in PATH: %w", err)
	}
	if err := ensureCommandAvailable("ffprobe"); err != nil {
		return nil, nil, fmt.Errorf("audio file exceeds ASR upstream limit and requires auto split, but ffprobe is not available in PATH: %w", err)
	}

	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return nil, nil, fmt.Errorf("stat local audio file %s: %w", localPath, err)
	}

	durationSeconds, err := probeAudioDuration(ctx, localPath)
	if err != nil {
		return nil, nil, err
	}
	if durationSeconds <= 0 {
		return nil, nil, fmt.Errorf("ffprobe returned invalid duration for %s", localPath)
	}

	workingDir, err := os.MkdirTemp(filepath.Dir(localPath), "asr-split-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create split workspace: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(workingDir)
	}

	segmentDuration := estimateSegmentDurationSeconds(fileInfo.Size(), c.maxAudioSizeBytes(), durationSeconds)
	for attempt := 0; attempt < maxSplitAttempts; attempt++ {
		if err := clearDirectoryFiles(workingDir); err != nil {
			cleanup()
			return nil, nil, err
		}

		segmentPaths, largestSegmentSize, err := splitAudioWithFFmpeg(ctx, localPath, workingDir, segmentDuration)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		if len(segmentPaths) == 0 {
			cleanup()
			return nil, nil, fmt.Errorf("ffmpeg did not produce any audio segments")
		}
		if largestSegmentSize <= c.maxAudioSizeBytes() {
			return segmentPaths, cleanup, nil
		}

		nextDuration := segmentDuration * (float64(c.maxAudioSizeBytes()) / float64(largestSegmentSize)) * splitSafetyRatio
		if nextDuration >= segmentDuration {
			nextDuration = segmentDuration / 2
		}
		if nextDuration < minSplitDurationSeconds {
			cleanup()
			return nil, nil, fmt.Errorf("failed to split audio under %d MB after %d attempts", c.maxAudioSizeMB, attempt+1)
		}
		segmentDuration = nextDuration
	}

	cleanup()
	return nil, nil, fmt.Errorf("failed to split audio under %d MB after %d attempts", c.maxAudioSizeMB, maxSplitAttempts)
}

func ensureCommandAvailable(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return err
	}
	return nil
}

func probeAudioDuration(ctx context.Context, localPath string) (float64, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		localPath,
	)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe %s: %w", localPath, err)
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(output, &probe); err != nil {
		return 0, fmt.Errorf("decode ffprobe output: %w", err)
	}

	durationSeconds, err := strconv.ParseFloat(strings.TrimSpace(probe.Format.Duration), 64)
	if err != nil {
		return 0, fmt.Errorf("parse ffprobe duration: %w", err)
	}

	return durationSeconds, nil
}

func estimateSegmentDurationSeconds(fileSizeBytes, maxSizeBytes int64, durationSeconds float64) float64 {
	if fileSizeBytes <= 0 || maxSizeBytes <= 0 || durationSeconds <= 0 {
		return minSplitDurationSeconds
	}

	ratio := (float64(maxSizeBytes) / float64(fileSizeBytes)) * splitSafetyRatio
	segmentDuration := durationSeconds * ratio
	if segmentDuration < minSplitDurationSeconds {
		return minSplitDurationSeconds
	}
	return math.Max(segmentDuration, minSplitDurationSeconds)
}

func splitAudioWithFFmpeg(ctx context.Context, inputPath, workingDir string, segmentDuration float64) ([]string, int64, error) {
	outputPattern := filepath.Join(workingDir, "chunk-%03d"+filepath.Ext(inputPath))
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", inputPath,
		"-f", "segment",
		"-segment_time", strconv.FormatFloat(segmentDuration, 'f', 3, 64),
		"-reset_timestamps", "1",
		"-c", "copy",
		outputPattern,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, 0, fmt.Errorf("ffmpeg split failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	entries, err := filepath.Glob(filepath.Join(workingDir, "chunk-*"+filepath.Ext(inputPath)))
	if err != nil {
		return nil, 0, fmt.Errorf("list split audio files: %w", err)
	}
	sort.Strings(entries)

	var largestSegmentSize int64
	for _, entry := range entries {
		info, err := os.Stat(entry)
		if err != nil {
			return nil, 0, fmt.Errorf("stat split audio file %s: %w", entry, err)
		}
		if info.Size() > largestSegmentSize {
			largestSegmentSize = info.Size()
		}
	}

	return entries, largestSegmentSize, nil
}

func clearDirectoryFiles(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read split workspace %s: %w", dir, err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return fmt.Errorf("clear split workspace %s: %w", dir, err)
		}
	}
	return nil
}
