package rulescatalog

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// parseMarkdownBody scans a rules markdown document and extracts the 9-column
// rule rows plus the H1 title. The expected header columns are:
//
//	规则类型 | 错误模式 (pattern) | 正确写法 (replacement) | 匹配方式 |
//	优先级 | 冲突组 | 启用 | 示例 | 备注
//
// Rows whose pattern column is empty or whose 规则类型 cell duplicates the
// pattern (placeholder rows in 00-评分规则与说明.md) are silently skipped, so
// the meta document does not pollute the rules export.
func parseMarkdownBody(sourcePath string, content []byte) (title string, rules []SectionRule) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	currentSub := ""
	headerSeen := false
	separatorSeen := false
	ruleIndex := 0
	rules = make([]SectionRule, 0, 64)

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
			if isRuleHeader(cells) {
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

		rule, ok := rowToRule(sourcePath, ruleIndex, cells, currentSub)
		if !ok {
			continue
		}
		rules = append(rules, rule)
		ruleIndex++
	}
	return title, rules
}

func splitMarkdownRow(line string) []string {
	trimmed := strings.Trim(line, "|")
	parts := splitMarkdownTableCells(trimmed)
	cells := make([]string, len(parts))
	for i, part := range parts {
		cells[i] = strings.TrimSpace(part)
	}
	return cells
}

func splitMarkdownTableCells(row string) []string {
	parts := []string{}
	var current strings.Builder
	escaped := false
	for _, value := range row {
		if escaped {
			if value != '|' {
				current.WriteRune('\\')
			}
			current.WriteRune(value)
			escaped = false
			continue
		}
		if value == '\\' {
			escaped = true
			continue
		}
		if value == '|' {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(value)
	}
	if escaped {
		current.WriteRune('\\')
	}
	parts = append(parts, current.String())
	return parts
}

func isRuleHeader(cells []string) bool {
	hasPattern := false
	hasReplacement := false
	for _, cell := range cells {
		normalized := strings.ToLower(cell)
		switch {
		case strings.HasPrefix(cell, "错误模式") || normalized == "pattern":
			hasPattern = true
		case strings.HasPrefix(cell, "正确写法") || normalized == "replacement":
			hasReplacement = true
		}
	}
	return hasPattern && hasReplacement
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

func rowToRule(sourcePath string, idx int, cells []string, subsection string) (SectionRule, bool) {
	matchType := normaliseMatchType(cleanCell(cells[3]))
	rule := SectionRule{
		Key:             fmt.Sprintf("%s#%04d", sourcePath, idx),
		Category:        cleanCell(cells[0]),
		Pattern:         normalizeRuleCatalogPattern(cleanCell(cells[1]), matchType),
		Replacement:     cleanCell(cells[2]),
		MatchType:       matchType,
		Priority:        parsePriorityCell(cells[4]),
		ConflictGroup:   cleanCell(cells[5]),
		Enabled:         parseEnabledCell(cells[6]),
		Example:         cleanCell(cells[7]),
		Notes:           cleanCell(cells[8]),
		SubsectionTitle: subsection,
		SourcePath:      sourcePath,
	}
	if rule.Pattern == "" && !ruleMatchTypeAllowsEmptyPattern(rule.MatchType) {
		return SectionRule{}, false
	}
	// Skip self-referential placeholder rows like "占位无修改" where pattern == replacement
	// AND the row is explicitly disabled with empty example: those are documentation, not rules.
	if !rule.Enabled && rule.Pattern == rule.Replacement && rule.Example == "占位" {
		return SectionRule{}, false
	}
	if rule.Priority == 0 {
		rule.Priority = 100
	}
	return rule, true
}

func ruleMatchTypeAllowsEmptyPattern(matchType string) bool {
	return matchType == "number_normalize" || matchType == "hallucination_trim"
}

func normalizeRuleCatalogPattern(pattern string, matchType string) string {
	if matchType != "regex" {
		return pattern
	}
	return strings.ReplaceAll(pattern, `\-`, "-")
}

func cleanCell(value string) string {
	v := strings.TrimSpace(value)
	if v == "—" || v == "-" {
		return ""
	}
	return v
}

func parsePriorityCell(value string) int {
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

func parseEnabledCell(value string) bool {
	v := strings.ToLower(cleanCell(value))
	switch v {
	case "", "是", "yes", "y", "true", "1", "启用", "on":
		return true
	case "否", "no", "n", "false", "0", "关闭", "off":
		return false
	}
	return true
}

func normaliseMatchType(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "regex", "正则", "regexp":
		return "regex"
	case "number_normalize", "number-normalize", "数字归一", "数值归一":
		return "number_normalize"
	case "hallucination_trim", "hallucination-trim", "幻觉裁剪", "幻觉去尾", "重复裁剪":
		return "hallucination_trim"
	case "", "literal", "字面", "exact", "原文":
		return "literal"
	}
	return "literal"
}
