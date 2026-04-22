package voiceintent

import (
	"math"
	"strings"
)

const commandMatchThreshold = 0.72

type commandCandidate struct {
	CommandID  uint64
	GroupKey   string
	Intent     string
	Label      string
	Utterance  string
	Normalized string
	SortOrder  int
}

func MatchCatalog(inputText string, catalog Catalog) Result {
	variants := buildCommandTextVariants(inputText)
	if len(variants) == 0 || len(catalog.Commands) == 0 {
		return Result{Matched: false, Reason: "未命中有效指令"}
	}

	candidates := buildCommandCandidates(catalog)
	if len(candidates) == 0 {
		return Result{Matched: false, Reason: "未命中有效指令"}
	}

	var best *commandCandidate
	bestScore := 0.0
	for index := range candidates {
		candidate := &candidates[index]
		score := 0.0
		for _, variant := range variants {
			score = math.Max(score, scoreNormalizedMatch(variant, candidate.Normalized))
		}
		if score > bestScore || (score == bestScore && best != nil && len(candidate.Normalized) > len(best.Normalized)) {
			bestScore = score
			best = candidate
		}
	}

	if best == nil || bestScore < commandMatchThreshold {
		return Result{Matched: false, Reason: "未命中有效指令"}
	}

	reason := "已匹配近义指令：" + best.Label
	if bestScore >= 0.95 {
		reason = "已命中指令：" + best.Label
	}

	return Result{
		Matched:    true,
		Intent:     best.Intent,
		GroupKey:   best.GroupKey,
		CommandID:  best.CommandID,
		Confidence: roundConfidence(bestScore),
		Reason:     reason,
		RawOutput:  best.Utterance,
	}
}

func buildCommandCandidates(catalog Catalog) []commandCandidate {
	seen := map[string]struct{}{}
	items := make([]commandCandidate, 0, len(catalog.Commands)*2)
	for commandIndex, command := range catalog.Commands {
		label := strings.TrimSpace(command.Label)
		for utteranceIndex, utterance := range command.Utterances {
			normalized := normalizeLooseText(utterance)
			if normalized == "" {
				continue
			}
			dedupeKey := command.GroupKey + ":" + command.Intent + ":" + normalized
			if _, ok := seen[dedupeKey]; ok {
				continue
			}
			seen[dedupeKey] = struct{}{}
			candidateLabel := label
			if candidateLabel == "" {
				candidateLabel = strings.TrimSpace(utterance)
			}
			items = append(items, commandCandidate{
				CommandID:  command.EntryID,
				GroupKey:   command.GroupKey,
				Intent:     command.Intent,
				Label:      candidateLabel,
				Utterance:  utterance,
				Normalized: normalized,
				SortOrder:  commandIndex*100 + utteranceIndex,
			})
		}
	}

	slicesSortStable(items)
	return items
}

func slicesSortStable(items []commandCandidate) {
	for i := 1; i < len(items); i++ {
		current := items[i]
		j := i - 1
		for ; j >= 0; j-- {
			if runeLen(items[j].Normalized) > runeLen(current.Normalized) {
				break
			}
			if runeLen(items[j].Normalized) == runeLen(current.Normalized) && items[j].SortOrder <= current.SortOrder {
				break
			}
			items[j+1] = items[j]
		}
		items[j+1] = current
	}
}

func buildCommandTextVariants(value string) []string {
	variants := map[string]struct{}{}
	base := normalizeLooseText(value)
	if base == "" {
		return nil
	}

	enqueue := func(candidate string) {
		normalized := normalizeLooseText(candidate)
		if normalized != "" {
			variants[normalized] = struct{}{}
		}
	}

	enqueue(base)
	enqueue(strings.TrimPrefix(base, "请你"))
	enqueue(strings.TrimPrefix(base, "请帮我"))
	enqueue(strings.TrimPrefix(base, "帮我"))
	enqueue(strings.TrimPrefix(base, "麻烦你"))
	enqueue(strings.TrimPrefix(base, "麻烦"))
	enqueue(strings.TrimPrefix(base, "给我"))
	enqueue(strings.TrimPrefix(base, "请"))
	for _, suffix := range []string{"吧", "呀", "啊", "呢", "啦", "嘛", "哦"} {
		enqueue(strings.TrimSuffix(base, suffix))
	}
	for _, prefix := range []string{"切换到", "切换成", "切到", "切成", "改成", "改到", "进入", "开始"} {
		enqueue(strings.TrimPrefix(base, prefix))
		for _, polite := range []string{"请你", "请帮我", "帮我", "麻烦你", "麻烦", "给我", "请"} {
			enqueue(strings.TrimPrefix(strings.TrimPrefix(base, polite), prefix))
		}
	}

	result := make([]string, 0, len(variants))
	for item := range variants {
		result = append(result, item)
	}
	return result
}

func normalizeLooseText(value string) string {
	replacer := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\n", "",
		"，", "",
		",", "",
		"。", "",
		".", "",
		"！", "",
		"!", "",
		"？", "",
		"?", "",
		"、", "",
		"-", "",
		"_", "",
		"(", "",
		")", "",
		"（", "",
		"）", "",
		"：", "",
		":", "",
		"；", "",
		";", "",
		"【", "",
		"】", "",
		"[", "",
		"]", "",
		"\"", "",
		"'", "",
		"“", "",
		"”", "",
		"‘", "",
		"’", "",
	)
	return replacer.Replace(strings.ToLower(strings.TrimSpace(value)))
}

func scoreNormalizedMatch(text string, candidate string) float64 {
	if text == "" || candidate == "" {
		return 0
	}
	if text == candidate {
		return 1
	}
	if strings.Contains(text, candidate) {
		return math.Min(0.99, 0.92+math.Min(float64(runeLen(candidate))/float64(runeLen(text)), 1)*0.07)
	}
	if runeLen(text) >= 2 && strings.Contains(candidate, text) {
		return 0.84 + math.Min(float64(runeLen(text))/float64(runeLen(candidate)), 1)*0.08
	}
	if absInt(runeLen(text)-runeLen(candidate)) > 4 {
		return 0
	}
	distance := levenshteinDistance(text, candidate)
	similarity := 1 - float64(distance)/float64(maxInt(runeLen(text), runeLen(candidate)))
	if similarity >= commandMatchThreshold {
		return similarity * 0.9
	}
	return 0
}

func levenshteinDistance(left string, right string) int {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	rows := len(leftRunes) + 1
	cols := len(rightRunes) + 1
	matrix := make([][]int, rows)
	for row := range matrix {
		matrix[row] = make([]int, cols)
		matrix[row][0] = row
	}
	for col := 0; col < cols; col++ {
		matrix[0][col] = col
	}
	for row := 1; row < rows; row++ {
		for col := 1; col < cols; col++ {
			cost := 1
			if leftRunes[row-1] == rightRunes[col-1] {
				cost = 0
			}
			matrix[row][col] = minInt(
				matrix[row-1][col]+1,
				matrix[row][col-1]+1,
				matrix[row-1][col-1]+cost,
			)
		}
	}
	return matrix[rows-1][cols-1]
}

func runeLen(value string) int {
	return len([]rune(value))
}

func roundConfidence(value float64) float64 {
	return math.Round(value*100) / 100
}

func minInt(values ...int) int {
	best := values[0]
	for _, value := range values[1:] {
		if value < best {
			best = value
		}
	}
	return best
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
