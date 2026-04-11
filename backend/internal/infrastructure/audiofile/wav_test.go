package audiofile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWritePCM16MonoWAV(t *testing.T) {
	t.Helper()

	outputPath := filepath.Join(t.TempDir(), "stream.wav")
	payload := make([]byte, 16000*2)
	if err := WritePCM16MonoWAV(outputPath, payload, 16000); err != nil {
		t.Fatalf("WritePCM16MonoWAV returned error: %v", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("stat wav output: %v", err)
	}
	if info.Size() <= 44 {
		t.Fatalf("expected wav file larger than header, got %d", info.Size())
	}

	duration, err := ProbeDuration(context.Background(), outputPath)
	if err != nil {
		t.Fatalf("ProbeDuration returned error: %v", err)
	}
	if duration <= 0.9 || duration >= 1.1 {
		t.Fatalf("expected duration close to 1 second, got %v", duration)
	}
}
