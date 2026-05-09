package terminology

import (
	"context"
	"strings"

	domain "github.com/lgt/asr/internal/domain/terminology"
)

// HotwordProvider derives upstream ASR hotwords from terminology entries.
type HotwordProvider struct {
	entryRepo domain.EntryRepository
}

func NewHotwordProvider(entryRepo domain.EntryRepository) *HotwordProvider {
	return &HotwordProvider{entryRepo: entryRepo}
}

func (p *HotwordProvider) HotwordsForDict(ctx context.Context, dictID uint64) ([]string, error) {
	if p == nil || p.entryRepo == nil || dictID == 0 {
		return nil, nil
	}

	entries, err := p.entryRepo.ListByDict(ctx, dictID)
	if err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	hotwords := make([]string, 0, len(entries))
	appendWord := func(value string) {
		word := strings.TrimSpace(value)
		if word == "" {
			return
		}
		if _, ok := seen[word]; ok {
			return
		}
		seen[word] = struct{}{}
		hotwords = append(hotwords, word)
	}

	for _, entry := range entries {
		appendWord(entry.CorrectTerm)
		for _, wrongVariant := range entry.WrongVariants {
			appendWord(wrongVariant)
		}
	}

	return hotwords, nil
}
