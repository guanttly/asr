package diarization

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var anonymousSpeakerLabelPattern = regexp.MustCompile(`(?i)^(?:speaker|spk)[_-]?(\d+)$`)

func AnonymousSpeakerLabelsUseZeroBased(labels []string) bool {
	for _, label := range labels {
		index, ok := parseAnonymousSpeakerIndex(label)
		if ok && index == 0 {
			return true
		}
	}
	return false
}

func NormalizeAnonymousSpeakerLabel(label string, zeroBased bool) string {
	trimmed := strings.TrimSpace(label)
	index, ok := parseAnonymousSpeakerIndex(trimmed)
	if !ok {
		return trimmed
	}
	if zeroBased {
		index += 1
	}
	if index <= 0 {
		index = 1
	}
	return fmt.Sprintf("说话人%d", index)
}

func parseAnonymousSpeakerIndex(label string) (int, bool) {
	matches := anonymousSpeakerLabelPattern.FindStringSubmatch(strings.TrimSpace(label))
	if len(matches) != 2 {
		return 0, false
	}
	index, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}
	return index, true
}
