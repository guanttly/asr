package nlpengine

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	domain "github.com/lgt/asr/internal/domain/terminology"
	"gorm.io/gorm"
)

// RuleAwareDictRepository is the subset required by the corrector.
type RuleAwareDictRepository interface {
	GetByID(ctx context.Context, id uint64) (*domain.TermDict, error)
}

// RuleAwareEntryRepository is the subset required by the corrector.
type RuleAwareEntryRepository interface {
	ListByDict(ctx context.Context, dictID uint64) ([]domain.TermEntry, error)
}

// Corrector applies terminology rules and near-term replacements.
type Corrector struct {
	dicts   RuleAwareDictRepository
	entries RuleAwareEntryRepository
	rules   domain.RuleRepository
}

// NewCorrector creates a new correction engine.
func NewCorrector(dicts RuleAwareDictRepository, entries RuleAwareEntryRepository, rules domain.RuleRepository) *Corrector {
	return &Corrector{dicts: dicts, entries: entries, rules: rules}
}

// Correct applies dictionary-owned rules first, then configured wrong variants.
func (corrector *Corrector) Correct(ctx context.Context, dictID *uint64, text string) (string, map[string][]string, error) {
	corrections := map[string][]string{
		"rules":  {},
		"terms":  {},
		"layer1": {}, // legacy response key for exact replacements
		"layer2": {}, // legacy response key for regex/structured rules
	}

	if dictID == nil {
		return text, corrections, nil
	}

	ruleProcessingEnabled := true
	textReplacementEnabled := true
	if corrector.dicts != nil {
		dict, err := corrector.dicts.GetByID(ctx, *dictID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return text, corrections, nil
			}
			return "", nil, err
		}
		ruleProcessingEnabled = dict.RuleProcessingEnabled
		textReplacementEnabled = dict.TextReplacementEnabled
	}

	corrected := text
	if ruleProcessingEnabled {
		rules, err := corrector.rules.ListByDict(ctx, *dictID)
		if err != nil {
			return "", nil, err
		}

		for _, rule := range rules {
			if !rule.Enabled {
				continue
			}
			next, applied, err := applyCorrectionRule(corrected, rule)
			if err != nil {
				return "", nil, err
			}
			if len(applied) == 0 {
				continue
			}
			corrected = next
			corrections["rules"] = append(corrections["rules"], applied...)
			corrections["layer2"] = append(corrections["layer2"], applied...)
		}
	}

	if textReplacementEnabled {
		entries, err := corrector.entries.ListByDict(ctx, *dictID)
		if err != nil {
			return "", nil, err
		}

		for _, entry := range entries {
			for _, wrong := range entry.WrongVariants {
				if wrong == "" || !strings.Contains(corrected, wrong) {
					continue
				}
				count := strings.Count(corrected, wrong)
				corrected = strings.ReplaceAll(corrected, wrong, entry.CorrectTerm)
				entryCorrection := wrong + "->" + entry.CorrectTerm + "(" + strconv.Itoa(count) + ")"
				corrections["terms"] = append(corrections["terms"], entryCorrection)
				corrections["layer1"] = append(corrections["layer1"], entryCorrection)
			}
		}
	}

	return corrected, corrections, nil
}

func applyCorrectionRule(text string, rule domain.CorrectionRule) (string, []string, error) {
	switch rule.MatchType {
	case domain.RuleMatchRegex:
		if strings.TrimSpace(rule.Pattern) == "" {
			return text, nil, nil
		}
		compiled, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return text, nil, err
		}
		matches := compiled.FindAllString(text, -1)
		if len(matches) == 0 {
			return text, nil, nil
		}
		replacement := normalizeRegexReplacement(rule.Replacement)
		return compiled.ReplaceAllString(text, replacement), []string{rule.Pattern + "=>" + rule.Replacement + "(" + strconv.Itoa(len(matches)) + ")"}, nil
	case domain.RuleMatchNumberNormalize:
		return normalizeSpokenNumbers(text)
	case domain.RuleMatchHallucinationTrim:
		return trimHallucinatedTranscript(text)
	default:
		if rule.Pattern == "" || !strings.Contains(text, rule.Pattern) {
			return text, nil, nil
		}
		count := strings.Count(text, rule.Pattern)
		return strings.ReplaceAll(text, rule.Pattern, rule.Replacement), []string{rule.Pattern + "->" + rule.Replacement + "(" + strconv.Itoa(count) + ")"}, nil
	}
}

func normalizeSpokenNumbers(text string) (string, []string, error) {
	textRunes := []rune(text)
	var builder strings.Builder
	applied := []string{}

	for index := 0; index < len(textRunes); {
		if replacement, endIndex, ok := parseDimensionExpression(textRunes, index); ok {
			original := string(textRunes[index:endIndex])
			builder.WriteString(replacement)
			applied = append(applied, original+"->"+replacement)
			index = endIndex
			continue
		}

		if replacement, endIndex, ok := parseDecimalExpression(textRunes, index); ok {
			original := string(textRunes[index:endIndex])
			builder.WriteString(replacement)
			applied = append(applied, original+"->"+replacement)
			index = endIndex
			continue
		}

		builder.WriteRune(textRunes[index])
		index++
	}

	return builder.String(), applied, nil
}

func normalizeRegexReplacement(replacement string) string {
	runes := []rune(replacement)
	var builder strings.Builder
	for index := 0; index < len(runes); {
		if runes[index] != '$' || index+1 >= len(runes) || !isASCIIDigitRune(runes[index+1]) {
			builder.WriteRune(runes[index])
			index++
			continue
		}

		digitStart := index + 1
		digitEnd := digitStart
		for digitEnd < len(runes) && isASCIIDigitRune(runes[digitEnd]) {
			digitEnd++
		}
		if digitEnd < len(runes) && (unicode.IsLetter(runes[digitEnd]) || runes[digitEnd] == '_') {
			builder.WriteString("${")
			builder.WriteString(string(runes[digitStart:digitEnd]))
			builder.WriteRune('}')
			index = digitEnd
			continue
		}

		builder.WriteString(string(runes[index:digitEnd]))
		index = digitEnd
	}
	return builder.String()
}

func isASCIIDigitRune(value rune) bool {
	return value >= '0' && value <= '9'
}

func trimHallucinatedTranscript(text string) (string, []string, error) {
	corrected := text
	applied := []string{}

	if next, ok := trimRepeatedBoilerplate(corrected); ok {
		corrected = next
		applied = append(applied, "hallucination_trim:boilerplate_tail")
	}
	if next, ok := trimRepeatedClosingTail(corrected); ok {
		corrected = next
		applied = append(applied, "hallucination_trim:repeated_tail")
	}
	if next, dropped := dropRunawayRepeatedClauses(corrected); dropped > 0 {
		corrected = next
		applied = append(applied, "hallucination_trim:repeated_clauses("+strconv.Itoa(dropped)+")")
	}

	if corrected == text {
		return text, nil, nil
	}
	return corrected, applied, nil
}

func trimRepeatedBoilerplate(text string) (string, bool) {
	anchors := []string{
		"本报告为急诊夜班临时报告",
		"如有重要变更，我们会及时告知，请确认就诊登记电话号码正确",
	}
	endings := []string{
		"保持手机通畅。",
		"保持手机通畅！",
		"保持手机通畅",
		"以正式报告为准！",
		"以正式报告为准。",
		"以正式报告为准",
	}

	for _, anchor := range anchors {
		first := strings.Index(text, anchor)
		if first < 0 {
			continue
		}
		secondRelative := strings.Index(text[first+len(anchor):], anchor)
		if secondRelative < 0 {
			continue
		}
		second := first + len(anchor) + secondRelative
		trimEnd := first + len(anchor)
		for _, ending := range endings {
			endingRelative := strings.Index(text[first:], ending)
			if endingRelative < 0 {
				continue
			}
			candidateEnd := first + endingRelative + len(ending)
			if candidateEnd <= second {
				trimEnd = candidateEnd
				break
			}
		}
		if trimEnd < len(text) {
			return strings.TrimSpace(text[:trimEnd]), true
		}
	}
	return text, false
}

func trimRepeatedClosingTail(text string) (string, bool) {
	textRunes := []rune(text)
	if len(textRunes) < 80 {
		return text, false
	}

	maxSuffixLen := len(textRunes) / 2
	if maxSuffixLen > 240 {
		maxSuffixLen = 240
	}
	for suffixLen := maxSuffixLen; suffixLen >= 32; suffixLen-- {
		suffix := string(textRunes[len(textRunes)-suffixLen:])
		if !looksLikeClosingTail(suffix) {
			continue
		}
		prefix := string(textRunes[:len(textRunes)-suffixLen])
		previous := strings.LastIndex(prefix, suffix)
		if previous < 0 {
			continue
		}
		keepEnd := previous + len(suffix)
		if keepEnd >= len(prefix) {
			continue
		}
		return strings.TrimSpace(prefix[:keepEnd]), true
	}
	return text, false
}

func looksLikeClosingTail(value string) bool {
	anchors := []string{
		"请结合临床",
		"随诊复查",
		"临床随诊",
		"以正式报告为准",
		"保持手机通畅",
	}
	for _, anchor := range anchors {
		if strings.Contains(value, anchor) {
			return true
		}
	}
	return false
}

func dropRunawayRepeatedClauses(text string) (string, int) {
	segments := splitTranscriptSegments(text)
	if len(segments) < 8 {
		return text, 0
	}

	counts := make(map[string]int)
	for _, segment := range segments {
		normalized := normalizeRepeatedClause(segment)
		if normalized == "" {
			continue
		}
		counts[normalized]++
	}

	var builder strings.Builder
	seen := make(map[string]int)
	dropped := 0
	for _, segment := range segments {
		normalized := normalizeRepeatedClause(segment)
		if shouldDropRepeatedClause(normalized, counts[normalized], seen[normalized]) {
			seen[normalized]++
			dropped++
			continue
		}
		if normalized != "" {
			seen[normalized]++
		}
		builder.WriteString(segment)
	}
	if dropped == 0 {
		return text, 0
	}
	return strings.TrimSpace(builder.String()), dropped
}

func splitTranscriptSegments(text string) []string {
	segments := []string{}
	start := 0
	for index, value := range text {
		if !isTranscriptSegmentDelimiter(value) {
			continue
		}
		end := index + len(string(value))
		segments = append(segments, text[start:end])
		start = end
	}
	if start < len(text) {
		segments = append(segments, text[start:])
	}
	return segments
}

func isTranscriptSegmentDelimiter(value rune) bool {
	switch value {
	case '。', '；', ';', '\n':
		return true
	}
	return false
}

var leadingClauseNumberPattern = regexp.MustCompile(`^[第]?[零〇一二三四五六七八九十百千万两\d]+[、.．，,：:]+`)

func normalizeRepeatedClause(segment string) string {
	normalized := strings.TrimSpace(segment)
	normalized = strings.Trim(normalized, " \t\r\n，,。；;：:、.．")
	for {
		next := leadingClauseNumberPattern.ReplaceAllString(normalized, "")
		if next == normalized {
			break
		}
		normalized = strings.TrimSpace(next)
	}

	var builder strings.Builder
	for _, value := range normalized {
		if isIgnorableClauseRune(value) {
			continue
		}
		builder.WriteRune(value)
	}
	normalized = builder.String()
	if len([]rune(normalized)) < 8 {
		return ""
	}
	return normalized
}

func isIgnorableClauseRune(value rune) bool {
	switch value {
	case ' ', '\t', '\r', '\n', '，', ',', '。', '；', ';', '：', ':', '、', '.', '．':
		return true
	}
	return false
}

func shouldDropRepeatedClause(normalized string, totalCount, seenCount int) bool {
	if normalized == "" || seenCount == 0 {
		return false
	}
	length := len([]rune(normalized))
	if length >= 12 && totalCount >= 4 {
		return true
	}
	return length >= 8 && totalCount >= 6
}

func parseDimensionExpression(textRunes []rune, startIndex int) (string, int, bool) {
	firstValue, firstEnd, _, ok := parseNumberPhrase(textRunes, startIndex)
	if !ok {
		return "", startIndex, false
	}
	separatorEnd, ok := parseDimensionSeparator(textRunes, firstEnd)
	if !ok {
		return "", startIndex, false
	}
	secondValue, secondEnd, _, ok := parseNumberPhrase(textRunes, separatorEnd)
	if !ok {
		return "", startIndex, false
	}
	unit, unitEnd, ok := parseDimensionUnit(textRunes, secondEnd)
	if !ok {
		return "", startIndex, false
	}
	return formatSpokenNumber(firstValue) + "x" + formatSpokenNumber(secondValue) + unit, unitEnd, true
}

func parseDecimalExpression(textRunes []rune, startIndex int) (string, int, bool) {
	if startIndex > 0 && textRunes[startIndex-1] == '第' {
		return "", startIndex, false
	}
	value, endIndex, hasDecimal, ok := parseNumberPhrase(textRunes, startIndex)
	if !ok || !hasDecimal {
		return "", startIndex, false
	}
	return formatSpokenNumber(value), endIndex, true
}

func parseNumberPhrase(textRunes []rune, startIndex int) (float64, int, bool, bool) {
	if startIndex >= len(textRunes) {
		return 0, startIndex, false, false
	}

	integerStart := startIndex
	integerEnd := startIndex
	for integerEnd < len(textRunes) && isIntegerNumberRune(textRunes[integerEnd]) {
		integerEnd++
	}
	if integerEnd == integerStart {
		return 0, startIndex, false, false
	}

	integerValue, ok := parseIntegerToken(textRunes[integerStart:integerEnd])
	if !ok {
		return 0, startIndex, false, false
	}

	if integerEnd >= len(textRunes) || textRunes[integerEnd] != '点' {
		return float64(integerValue), integerEnd, false, true
	}

	decimalStart := integerEnd + 1
	decimalEnd := decimalStart
	for decimalEnd < len(textRunes) && isDecimalNumberRune(textRunes[decimalEnd]) {
		decimalEnd++
	}
	if decimalEnd == decimalStart {
		return float64(integerValue), integerEnd, false, true
	}

	decimalDigits := strings.Builder{}
	for _, decimalRune := range textRunes[decimalStart:decimalEnd] {
		digit, ok := decimalDigit(decimalRune)
		if !ok {
			return 0, startIndex, false, false
		}
		decimalDigits.WriteString(strconv.Itoa(digit))
	}

	value, err := strconv.ParseFloat(strconv.FormatInt(integerValue, 10)+"."+decimalDigits.String(), 64)
	if err != nil {
		return 0, startIndex, false, false
	}
	return value, decimalEnd, true, true
}

func parseIntegerToken(token []rune) (int64, bool) {
	if len(token) == 0 {
		return 0, false
	}
	if containsChineseUnit(token) {
		return parseChineseUnitNumber(token)
	}

	digits := strings.Builder{}
	for _, tokenRune := range token {
		digit, ok := decimalDigit(tokenRune)
		if !ok {
			return 0, false
		}
		digits.WriteString(strconv.Itoa(digit))
	}
	value, err := strconv.ParseInt(digits.String(), 10, 64)
	return value, err == nil
}

func parseChineseUnitNumber(token []rune) (int64, bool) {
	var total int64
	var section int64
	var number int64

	for _, tokenRune := range token {
		if digit, ok := chineseDigit(tokenRune); ok {
			number = int64(digit)
			continue
		}

		unit, ok := chineseUnit(tokenRune)
		if !ok {
			return 0, false
		}
		if unit == 10000 || unit == 100000000 {
			if number > 0 {
				section += number
			}
			if section == 0 {
				section = 1
			}
			total += section * unit
			section = 0
			number = 0
			continue
		}
		if number == 0 {
			number = 1
		}
		section += number * unit
		number = 0
	}

	return total + section + number, true
}

func parseDimensionSeparator(textRunes []rune, startIndex int) (int, bool) {
	if startIndex >= len(textRunes) {
		return startIndex, false
	}
	if startIndex+1 < len(textRunes) && textRunes[startIndex] == '乘' && textRunes[startIndex+1] == '以' {
		return startIndex + 2, true
	}
	switch textRunes[startIndex] {
	case '乘', '×', 'x', 'X', '*':
		return startIndex + 1, true
	default:
		return startIndex, false
	}
}

func parseDimensionUnit(textRunes []rune, startIndex int) (string, int, bool) {
	units := []struct {
		spoken string
		paper  string
	}{
		{spoken: "毫米", paper: "mm"},
		{spoken: "厘米", paper: "cm"},
		{spoken: "公分", paper: "cm"},
		{spoken: "米", paper: "m"},
	}

	remaining := string(textRunes[startIndex:])
	for _, unit := range units {
		if strings.HasPrefix(remaining, unit.spoken) {
			return unit.paper, startIndex + len([]rune(unit.spoken)), true
		}
	}
	return "", startIndex, false
}

func formatSpokenNumber(value float64) string {
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func containsChineseUnit(token []rune) bool {
	for _, tokenRune := range token {
		if _, ok := chineseUnit(tokenRune); ok {
			return true
		}
	}
	return false
}

func isIntegerNumberRune(value rune) bool {
	if _, ok := decimalDigit(value); ok {
		return true
	}
	_, ok := chineseUnit(value)
	return ok
}

func isDecimalNumberRune(value rune) bool {
	_, ok := decimalDigit(value)
	return ok
}

func decimalDigit(value rune) (int, bool) {
	if value >= '0' && value <= '9' {
		return int(value - '0'), true
	}
	return chineseDigit(value)
}

func chineseDigit(value rune) (int, bool) {
	switch value {
	case '零', '〇', '幺':
		return 0, true
	case '一':
		return 1, true
	case '二', '两':
		return 2, true
	case '三':
		return 3, true
	case '四':
		return 4, true
	case '五':
		return 5, true
	case '六':
		return 6, true
	case '七':
		return 7, true
	case '八':
		return 8, true
	case '九':
		return 9, true
	default:
		return 0, false
	}
}

func chineseUnit(value rune) (int64, bool) {
	switch value {
	case '十':
		return 10, true
	case '百':
		return 100, true
	case '千':
		return 1000, true
	case '万':
		return 10000, true
	case '亿':
		return 100000000, true
	default:
		return 0, false
	}
}
