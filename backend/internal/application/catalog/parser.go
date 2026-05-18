package catalog

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// misrecSplit normalises the multi-variant cell ("肝→敢、感、刚") into a
// list. Splitters are common Chinese/English punctuation found in the catalog.
var misrecSplit = regexp.MustCompile(`[、,，;；/／|｜\n]+`)

// parseMarkdownBody parses a markdown document into the canonical 9-column
// terminology table rows, plus the H1 title for display purposes. It is
// intentionally tolerant: cells that don't look like terms (eg. auxiliary
// reference tables in 01-通用) are silently skipped.
func parseMarkdownBody(sourcePath string, content []byte) (title string, terms []SectionTerm) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	currentSub := ""
	headerSeen := false
	separatorSeen := false
	termIndex := 0
	terms = make([]SectionTerm, 0, 64)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if title == "" && strings.HasPrefix(trimmed, "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			currentSub = strings.TrimSpace(strings.TrimPrefix(trimmed, "##"))
			headerSeen = false
			separatorSeen = false
			continue
		}

		if !strings.HasPrefix(trimmed, "|") {
			if trimmed == "" {
				headerSeen = false
				separatorSeen = false
			}
			continue
		}

		cells := splitMarkdownRow(trimmed)
		if len(cells) < 9 {
			continue
		}

		if !headerSeen {
			if isTermHeader(cells) {
				headerSeen = true
			}
			continue
		}
		if !separatorSeen {
			if isSeparatorRow(cells) {
				separatorSeen = true
				continue
			}
			separatorSeen = true
		}

		term, ok := rowToTerm(sourcePath, termIndex, cells, currentSub)
		if !ok {
			continue
		}
		terms = append(terms, term)
		termIndex++
	}
	return title, terms
}

func splitMarkdownRow(line string) []string {
	trimmed := strings.Trim(line, "|")
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, part := range parts {
		cells[i] = strings.TrimSpace(part)
	}
	return cells
}

func isTermHeader(cells []string) bool {
	hasStandard := false
	hasLevel := false
	for _, cell := range cells {
		switch cell {
		case "标准术语":
			hasStandard = true
		case "等级":
			hasLevel = true
		}
	}
	return hasStandard && hasLevel
}

func isSeparatorRow(cells []string) bool {
	for _, cell := range cells {
		if cell == "" {
			continue
		}
		if !strings.ContainsAny(cell, "-:") {
			return false
		}
		for _, r := range cell {
			if r != '-' && r != ':' && r != ' ' {
				return false
			}
		}
	}
	return true
}

func rowToTerm(sourcePath string, idx int, cells []string, subsection string) (SectionTerm, bool) {
	term := SectionTerm{
		Key:             fmt.Sprintf("%s#%04d", sourcePath, idx),
		StandardTerm:    cleanCell(cells[0]),
		EnglishOrAbbr:   cleanCell(cells[1]),
		Pinyin:          cleanCell(cells[2]),
		MixedScore:      parseScoreCell(cells[3]),
		RareScore:       parseScoreCell(cells[4]),
		GlyphScore:      parseScoreCell(cells[5]),
		Level:           normaliseLevel(cleanCell(cells[6])),
		CommonMisrecs:   splitMisrecs(cells[7]),
		Notes:           cleanCell(cells[8]),
		SubsectionTitle: subsection,
		SourcePath:      sourcePath,
	}
	if term.StandardTerm == "" {
		return SectionTerm{}, false
	}
	return term, true
}

func cleanCell(value string) string {
	v := strings.TrimSpace(value)
	if v == "—" || v == "-" {
		return ""
	}
	return v
}

func parseScoreCell(value string) int {
	v := cleanCell(value)
	if v == "" {
		return 0
	}
	score, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return score
}

func normaliseLevel(value string) string {
	v := strings.ToUpper(strings.TrimSpace(value))
	switch {
	case strings.HasPrefix(v, "L1"):
		return "L1"
	case strings.HasPrefix(v, "L2"):
		return "L2"
	case strings.HasPrefix(v, "L3"):
		return "L3"
	}
	return ""
}

func splitMisrecs(raw string) []string {
	v := cleanCell(raw)
	if v == "" {
		return nil
	}
	parts := misrecSplit.Split(v, -1)
	items := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		token := cleanCell(part)
		if token == "" {
			continue
		}
		if idx := strings.Index(token, "→"); idx >= 0 {
			token = strings.TrimSpace(token[idx+len("→"):])
		}
		if token == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		items = append(items, token)
	}
	return items
}
