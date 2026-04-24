package diarization

import (
	"strings"
	"testing"
)

func TestSplitTranscriptByDurationsHandlesMoreSegmentsThanUnits(t *testing.T) {
	t.Parallel()

	parts := SplitTranscriptByDurations("甲。乙。", []float64{1, 1, 1, 1})
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d", len(parts))
	}
	if strings.Join(nonEmptyParts(parts), "") != "甲。乙。" {
		t.Fatalf("expected non-empty parts to preserve transcript order, got %+v", parts)
	}
	if countNonEmptyParts(parts) != 2 {
		t.Fatalf("expected exactly 2 non-empty parts, got %+v", parts)
	}
}

func TestSplitTranscriptByDurationsHandlesMoreSegmentsThanRunes(t *testing.T) {
	t.Parallel()

	parts := SplitTranscriptByDurations("你好", []float64{1, 1, 1, 1})
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d", len(parts))
	}
	if strings.Join(nonEmptyParts(parts), "") != "你好" {
		t.Fatalf("expected non-empty parts to preserve transcript order, got %+v", parts)
	}
	if countNonEmptyParts(parts) != 2 {
		t.Fatalf("expected exactly 2 non-empty parts, got %+v", parts)
	}
}

func nonEmptyParts(parts []string) []string {
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func countNonEmptyParts(parts []string) int {
	return len(nonEmptyParts(parts))
}
