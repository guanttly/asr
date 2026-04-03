package asr

import (
	"strings"
	"testing"

	domain "github.com/lgt/asr/internal/domain/asr"
)

func TestTaskProgressUsesSegmentCountsDuringProcessing(t *testing.T) {
	task := &domain.TranscriptionTask{
		Status:           domain.TaskStatusProcessing,
		SegmentTotal:     5,
		SegmentCompleted: 2,
	}

	percent, stage, message := taskProgress(task)
	if percent != 40 {
		t.Fatalf("expected percent 40, got %d", percent)
	}
	if stage != "processing" {
		t.Fatalf("expected stage processing, got %s", stage)
	}
	if !strings.Contains(message, "第 3/5 片处理中") {
		t.Fatalf("expected message to describe current segment, got %q", message)
	}
	if !strings.Contains(message, "已完成 2/5") {
		t.Fatalf("expected message to contain segment progress, got %q", message)
	}
}

func TestTaskProgressUsesSegmentCountsAfterCompletion(t *testing.T) {
	task := &domain.TranscriptionTask{
		Status:            domain.TaskStatusCompleted,
		PostProcessStatus: domain.PostProcessProcessing,
		SegmentTotal:      4,
		SegmentCompleted:  4,
	}

	percent, stage, message := taskProgress(task)
	if percent != 100 {
		t.Fatalf("expected percent 100, got %d", percent)
	}
	if stage != "postprocessing" {
		t.Fatalf("expected stage postprocessing, got %s", stage)
	}
	if !strings.Contains(message, "4/4") {
		t.Fatalf("expected message to contain segment progress, got %q", message)
	}
}
