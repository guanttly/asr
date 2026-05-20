package nlpengine

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"

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
		return compiled.ReplaceAllString(text, rule.Replacement), []string{rule.Pattern + "=>" + rule.Replacement + "(" + strconv.Itoa(len(matches)) + ")"}, nil
	case domain.RuleMatchNumberNormalize:
		return normalizeSpokenNumbers(text)
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
