package audiofile

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type ffprobeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// ProbeDuration returns the audio duration in seconds using ffprobe.
func ProbeDuration(ctx context.Context, localPath string) (float64, error) {
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
