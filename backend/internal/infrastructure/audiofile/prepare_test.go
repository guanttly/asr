package audiofile

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPrepareForProcessingConvertsNonWavInput(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available")
	}

	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.mp3")
	writeTestTone(t, inputPath)

	prepared, err := PrepareForProcessing(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("PrepareForProcessing returned error: %v", err)
	}
	if filepath.Ext(prepared.Path) != ".wav" {
		t.Fatalf("expected prepared wav path, got %s", prepared.Path)
	}
	if prepared.Duration <= 0 {
		t.Fatalf("expected positive duration, got %v", prepared.Duration)
	}
	if _, err := os.Stat(prepared.Path); err != nil {
		t.Fatalf("stat prepared audio: %v", err)
	}
	if _, err := os.Stat(inputPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected original source removed, stat err=%v", err)
	}
}

func writeTestTone(t *testing.T, outputPath string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-f", "lavfi",
		"-i", "sine=frequency=1000:duration=1",
		"-ar", "8000",
		"-ac", "1",
		outputPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate test audio: %v: %s", err, string(output))
	}
}
