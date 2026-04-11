package audiofile

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSplitForMaxBytes(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available")
	}

	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "long.wav")
	if err := writeTestWav(inputPath, 16000, 16*16000); err != nil {
		t.Fatalf("write input wav: %v", err)
	}

	workingDir := filepath.Join(tempDir, "split")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("mkdir split dir: %v", err)
	}

	segments, largestSize, err := SplitForMaxBytes(context.Background(), inputPath, workingDir, 500000)
	if err != nil {
		t.Fatalf("SplitForMaxBytes returned error: %v", err)
	}
	if len(segments) < 2 {
		t.Fatalf("expected multiple segments, got %d", len(segments))
	}
	if largestSize <= 0 {
		t.Fatalf("expected positive largest size, got %d", largestSize)
	}
	for _, segment := range segments {
		info, err := os.Stat(segment)
		if err != nil {
			t.Fatalf("stat segment %s: %v", segment, err)
		}
		if info.Size() > 500000 {
			t.Fatalf("expected segment %s under size limit, got %d", segment, info.Size())
		}
	}
}
