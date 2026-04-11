package audiofile

import (
	"context"
	"fmt"
	"math"
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

// SplitForMaxBytes splits an audio file into chunks that fit under maxSizeBytes.
func SplitForMaxBytes(ctx context.Context, inputPath, workingDir string, maxSizeBytes int64) ([]string, int64, error) {
	if strings.TrimSpace(inputPath) == "" {
		return nil, 0, fmt.Errorf("input audio file path is required")
	}
	if maxSizeBytes <= 0 {
		return nil, 0, fmt.Errorf("maxSizeBytes must be greater than 0")
	}
	if err := ensureCommandAvailable("ffmpeg"); err != nil {
		return nil, 0, fmt.Errorf("ffmpeg not available: %w", err)
	}
	if err := ensureCommandAvailable("ffprobe"); err != nil {
		return nil, 0, fmt.Errorf("ffprobe not available: %w", err)
	}

	fileInfo, err := os.Stat(inputPath)
	if err != nil {
		return nil, 0, fmt.Errorf("stat local audio file %s: %w", inputPath, err)
	}

	durationSeconds, err := ProbeDuration(ctx, inputPath)
	if err != nil {
		return nil, 0, err
	}
	if durationSeconds <= 0 {
		return nil, 0, fmt.Errorf("ffprobe returned invalid duration for %s", inputPath)
	}

	segmentDuration := estimateSegmentDurationSeconds(fileInfo.Size(), maxSizeBytes, durationSeconds)
	for attempt := 0; attempt < maxSplitAttempts; attempt++ {
		if err := clearDirectoryFiles(workingDir); err != nil {
			return nil, 0, err
		}

		segmentPaths, largestSegmentSize, err := splitAudioWithFFmpeg(ctx, inputPath, workingDir, segmentDuration)
		if err != nil {
			return nil, 0, err
		}
		if len(segmentPaths) == 0 {
			return nil, 0, fmt.Errorf("ffmpeg did not produce any audio segments")
		}
		if largestSegmentSize <= maxSizeBytes {
			return segmentPaths, largestSegmentSize, nil
		}

		nextDuration := segmentDuration * (float64(maxSizeBytes) / float64(largestSegmentSize)) * splitSafetyRatio
		if nextDuration >= segmentDuration {
			nextDuration = segmentDuration / 2
		}
		if nextDuration < minSplitDurationSeconds {
			return nil, 0, fmt.Errorf("failed to split audio under %d bytes after %d attempts", maxSizeBytes, attempt+1)
		}
		segmentDuration = nextDuration
	}

	return nil, 0, fmt.Errorf("failed to split audio under %d bytes after %d attempts", maxSizeBytes, maxSplitAttempts)
}

func ensureCommandAvailable(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return err
	}
	return nil
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
