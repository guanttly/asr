package audiofile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PreparedFile struct {
	Path     string
	Duration float64
}

// PrepareForProcessing normalizes audio into 16kHz mono WAV and probes duration.
func PrepareForProcessing(ctx context.Context, inputPath string) (*PreparedFile, error) {
	if strings.TrimSpace(inputPath) == "" {
		return nil, fmt.Errorf("input audio file path is required")
	}

	normalizedPath, err := normalizeForProcessing(ctx, inputPath)
	if err != nil {
		return nil, err
	}

	return &PreparedFile{
		Path:     normalizedPath,
		Duration: probeDurationBestEffort(ctx, normalizedPath),
	}, nil
}

func normalizeForProcessing(ctx context.Context, inputPath string) (string, error) {
	currentExt := strings.ToLower(filepath.Ext(inputPath))
	normalizedPath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".wav"
	renderPath := normalizedPath
	replaceOriginal := currentExt == ".wav"
	if replaceOriginal {
		renderPath = normalizedPath + ".normalized-tmp"
	}

	if err := NormalizeToWav16kMono(ctx, inputPath, renderPath); err != nil {
		if replaceOriginal {
			_ = os.Remove(renderPath)
		}
		return "", err
	}

	if replaceOriginal {
		if err := os.Rename(renderPath, normalizedPath); err != nil {
			_ = os.Remove(renderPath)
			return "", err
		}
		return normalizedPath, nil
	}

	_ = os.Remove(inputPath)
	return normalizedPath, nil
}

func probeDurationBestEffort(ctx context.Context, localPath string) float64 {
	duration, err := ProbeDuration(ctx, localPath)
	if err != nil {
		return 0
	}
	return duration
}
