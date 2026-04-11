package audiofile

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// NormalizeToWav16kMono rewrites an audio file into 16kHz mono PCM WAV.
func NormalizeToWav16kMono(ctx context.Context, inputPath, outputPath string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not available: %w", err)
	}

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", inputPath,
		"-ar", "16000",
		"-ac", "1",
		"-c:a", "pcm_s16le",
		outputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg normalize failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}
