package diarization

import (
	"strings"
	"unicode/utf8"
)

func SplitTranscriptByDurations(text string, durations []float64) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || len(durations) == 0 {
		return nil
	}

	units := transcriptTextUnits(trimmed)
	if len(units) == 0 {
		return []string{trimmed}
	}
	if len(units) == 1 {
		return splitSingleUnitByDurations(units[0], durations)
	}

	normalizedDurations, totalDuration := normalizeDurations(durations)
	segmentEnds := buildSegmentProgressEnds(normalizedDurations, totalDuration)
	totalRunes := 0
	unitRunes := make([]int, len(units))
	for index, unit := range units {
		count := utf8.RuneCountInString(unit)
		unitRunes[index] = count
		totalRunes += count
	}
	if totalRunes <= 0 {
		return append([]string(nil), trimmed)
	}

	partUnits := make([][]string, len(normalizedDurations))
	consumedRunes := 0
	for index, unit := range units {
		count := unitRunes[index]
		if count <= 0 {
			continue
		}
		startProgress := float64(consumedRunes) / float64(totalRunes)
		consumedRunes += count
		endProgress := float64(consumedRunes) / float64(totalRunes)
		unitMidpoint := (startProgress + endProgress) / 2
		segmentIndex := locateSegmentIndex(unitMidpoint, segmentEnds)
		partUnits[segmentIndex] = append(partUnits[segmentIndex], unit)
	}

	parts := make([]string, len(normalizedDurations))
	for index, items := range partUnits {
		parts[index] = strings.TrimSpace(strings.Join(items, " "))
	}
	return parts
}

func normalizeDurations(durations []float64) ([]float64, float64) {
	normalized := make([]float64, len(durations))
	totalDuration := 0.0
	for index, duration := range durations {
		if duration > 0 {
			normalized[index] = duration
			totalDuration += duration
		}
	}
	if totalDuration > 0 {
		return normalized, totalDuration
	}
	for index := range normalized {
		normalized[index] = 1
	}
	return normalized, float64(len(normalized))
}

func transcriptTextUnits(text string) []string {
	normalized := strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n")
	if normalized == "" {
		return nil
	}
	lines := strings.Split(normalized, "\n")
	units := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		units = append(units, splitTextBySentence(trimmed)...)
	}
	return units
}

func splitTextBySentence(text string) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}
	parts := make([]string, 0)
	start := 0
	for index, char := range runes {
		if !strings.ContainsRune("。！？!?；;", char) {
			continue
		}
		segment := strings.TrimSpace(string(runes[start : index+1]))
		if segment != "" {
			parts = append(parts, segment)
		}
		start = index + 1
	}
	if start < len(runes) {
		segment := strings.TrimSpace(string(runes[start:]))
		if segment != "" {
			parts = append(parts, segment)
		}
	}
	if len(parts) == 0 {
		return []string{strings.TrimSpace(text)}
	}
	return parts
}

func splitSingleUnitByDurations(text string, durations []float64) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 || len(durations) == 0 {
		return nil
	}

	normalizedDurations, totalDuration := normalizeDurations(durations)
	segmentEnds := buildSegmentProgressEnds(normalizedDurations, totalDuration)
	partRunes := make([][]rune, len(normalizedDurations))
	totalRunes := len(runes)
	for index, char := range runes {
		runeMidpoint := (float64(index) + 0.5) / float64(totalRunes)
		segmentIndex := locateSegmentIndex(runeMidpoint, segmentEnds)
		partRunes[segmentIndex] = append(partRunes[segmentIndex], char)
	}

	parts := make([]string, len(normalizedDurations))
	for index, chars := range partRunes {
		parts[index] = strings.TrimSpace(string(chars))
	}
	return parts
}

func buildSegmentProgressEnds(durations []float64, totalDuration float64) []float64 {
	if len(durations) == 0 {
		return nil
	}
	ends := make([]float64, len(durations))
	accumulated := 0.0
	for index, duration := range durations {
		accumulated += duration
		if totalDuration > 0 {
			ends[index] = accumulated / totalDuration
		}
	}
	ends[len(ends)-1] = 1
	return ends
}

func locateSegmentIndex(progress float64, ends []float64) int {
	if len(ends) == 0 {
		return 0
	}
	for index, end := range ends {
		if progress <= end || index == len(ends)-1 {
			return index
		}
	}
	return len(ends) - 1
}
