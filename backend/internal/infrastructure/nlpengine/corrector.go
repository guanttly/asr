package nlpengine

import (
	"context"
	"strings"

	domain "github.com/lgt/asr/internal/domain/terminology"
)

// RuleAwareEntryRepository is the subset required by the corrector.
type RuleAwareEntryRepository interface {
	ListByDict(ctx context.Context, dictID uint64) ([]domain.TermEntry, error)
}

// Corrector provides a minimal three-layer correction pipeline.
type Corrector struct {
	entries RuleAwareEntryRepository
	rules   domain.RuleRepository
}

// NewCorrector creates a new correction engine.
func NewCorrector(entries RuleAwareEntryRepository, rules domain.RuleRepository) *Corrector {
	return &Corrector{entries: entries, rules: rules}
}

// Correct applies exact, fuzzy placeholder, and pinyin placeholder corrections.
func (c *Corrector) Correct(ctx context.Context, dictID *uint64, text string) (string, map[string][]string, error) {
	corrections := map[string][]string{
		"layer1": {},
		"layer2": {},
		"layer3": {},
	}

	if dictID == nil {
		return text, corrections, nil
	}

	entries, err := c.entries.ListByDict(ctx, *dictID)
	if err != nil {
		return "", nil, err
	}

	corrected := text
	for _, entry := range entries {
		for _, wrong := range entry.WrongVariants {
			if wrong == "" || !strings.Contains(corrected, wrong) {
				continue
			}
			corrected = strings.ReplaceAll(corrected, wrong, entry.CorrectTerm)
			corrections["layer1"] = append(corrections["layer1"], wrong+"->"+entry.CorrectTerm)
		}
	}

	rules, err := c.rules.ListByDict(ctx, *dictID)
	if err != nil {
		return "", nil, err
	}

	for _, rule := range rules {
		if !rule.Enabled || !strings.Contains(corrected, rule.Pattern) {
			continue
		}
		corrected = strings.ReplaceAll(corrected, rule.Pattern, rule.Replacement)
		layerKey := "layer2"
		if rule.Layer == domain.LayerPinyinSimilar {
			layerKey = "layer3"
		}
		corrections[layerKey] = append(corrections[layerKey], rule.Pattern+"->"+rule.Replacement)
	}

	return corrected, corrections, nil
}
