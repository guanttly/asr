package terminology

import (
	"context"
	"sort"
	"testing"

	domain "github.com/lgt/asr/internal/domain/terminology"
)

type termSeedRepoStub struct {
	seeded map[string]bool
}

func (r *termSeedRepoStub) IsSeeded(_ context.Context, key string) (bool, error) {
	return r.seeded[key], nil
}

func (r *termSeedRepoStub) MarkSeeded(_ context.Context, key string) error {
	if r.seeded == nil {
		r.seeded = map[string]bool{}
	}
	r.seeded[key] = true
	return nil
}

type termDictRepoStub struct {
	nextID  uint64
	items   map[uint64]*domain.TermDict
	deleted []uint64
}

func (r *termDictRepoStub) Create(_ context.Context, dict *domain.TermDict) error {
	if r.items == nil {
		r.items = map[uint64]*domain.TermDict{}
	}
	r.nextID++
	dict.ID = r.nextID
	copyItem := *dict
	r.items[dict.ID] = &copyItem
	return nil
}

func (r *termDictRepoStub) GetByID(_ context.Context, id uint64) (*domain.TermDict, error) {
	if item := r.items[id]; item != nil {
		copyItem := *item
		return &copyItem, nil
	}
	return nil, nil
}

func (r *termDictRepoStub) Update(_ context.Context, dict *domain.TermDict) error {
	copyItem := *dict
	r.items[dict.ID] = &copyItem
	return nil
}

func (r *termDictRepoStub) Delete(_ context.Context, id uint64) error {
	delete(r.items, id)
	r.deleted = append(r.deleted, id)
	return nil
}

func (r *termDictRepoStub) List(_ context.Context, offset, limit int) ([]*domain.TermDict, int64, error) {
	ids := make([]int, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	items := make([]*domain.TermDict, 0, len(ids))
	for _, id := range ids {
		copyItem := *r.items[uint64(id)]
		items = append(items, &copyItem)
	}
	total := int64(len(items))
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = len(items)
	}
	if offset >= len(items) {
		return []*domain.TermDict{}, total, nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], total, nil
}

type termEntryRepoStub struct {
	nextID  uint64
	items   map[uint64]domain.TermEntry
	deleted []uint64
}

func (r *termEntryRepoStub) BatchCreate(_ context.Context, entries []domain.TermEntry) error {
	if r.items == nil {
		r.items = map[uint64]domain.TermEntry{}
	}
	for _, entry := range entries {
		r.nextID++
		entry.ID = r.nextID
		r.items[entry.ID] = entry
	}
	return nil
}

func (r *termEntryRepoStub) GetByID(_ context.Context, id uint64) (*domain.TermEntry, error) {
	if item, ok := r.items[id]; ok {
		return &item, nil
	}
	return nil, nil
}

func (r *termEntryRepoStub) ListByDict(_ context.Context, dictID uint64) ([]domain.TermEntry, error) {
	items := make([]domain.TermEntry, 0)
	for _, item := range r.items {
		if item.DictID == dictID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (r *termEntryRepoStub) Update(_ context.Context, entry *domain.TermEntry) error {
	r.items[entry.ID] = *entry
	return nil
}

func (r *termEntryRepoStub) Delete(_ context.Context, id uint64) error {
	delete(r.items, id)
	r.deleted = append(r.deleted, id)
	return nil
}

type termRuleRepoStub struct {
	nextID  uint64
	items   map[uint64]domain.CorrectionRule
	deleted []uint64
}

func (r *termRuleRepoStub) Create(_ context.Context, rule *domain.CorrectionRule) error {
	if r.items == nil {
		r.items = map[uint64]domain.CorrectionRule{}
	}
	r.nextID++
	rule.ID = r.nextID
	r.items[rule.ID] = *rule
	return nil
}

func (r *termRuleRepoStub) BatchCreate(ctx context.Context, rules []domain.CorrectionRule) error {
	for i := range rules {
		if err := r.Create(ctx, &rules[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *termRuleRepoStub) GetByID(_ context.Context, id uint64) (*domain.CorrectionRule, error) {
	if item, ok := r.items[id]; ok {
		return &item, nil
	}
	return nil, nil
}

func (r *termRuleRepoStub) ListByDict(_ context.Context, dictID uint64) ([]domain.CorrectionRule, error) {
	items := make([]domain.CorrectionRule, 0)
	for _, item := range r.items {
		if item.DictID == dictID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (r *termRuleRepoStub) Update(_ context.Context, rule *domain.CorrectionRule) error {
	r.items[rule.ID] = *rule
	return nil
}

func (r *termRuleRepoStub) Delete(_ context.Context, id uint64) error {
	delete(r.items, id)
	r.deleted = append(r.deleted, id)
	return nil
}

func TestEnsureSeedDataKeepsExistingDeprecatedDictionaries(t *testing.T) {
	dicts := &termDictRepoStub{nextID: 2, items: map[uint64]*domain.TermDict{
		1: {ID: 1, Name: "庭审记录", Domain: "法律"},
		2: {ID: 2, Name: "写报告", Domain: "医疗"},
	}}
	entries := &termEntryRepoStub{nextID: 2, items: map[uint64]domain.TermEntry{
		1: {ID: 1, DictID: 1, CorrectTerm: "用户法律词"},
		2: {ID: 2, DictID: 2, CorrectTerm: "用户医疗词"},
	}}
	rules := &termRuleRepoStub{nextID: 2, items: map[uint64]domain.CorrectionRule{
		1: {ID: 1, DictID: 1, MatchType: domain.RuleMatchLiteral, Pattern: "旧", Replacement: "新"},
		2: {ID: 2, DictID: 2, MatchType: domain.RuleMatchLiteral, Pattern: "舒张亚", Replacement: "舒张压"},
	}}
	seeds := &termSeedRepoStub{}
	service := NewService(dicts, entries, rules, seeds)

	if err := service.EnsureSeedData(context.Background()); err != nil {
		t.Fatalf("EnsureSeedData returned error: %v", err)
	}

	if len(dicts.deleted) != 0 || len(entries.deleted) != 0 || len(rules.deleted) != 0 {
		t.Fatalf("expected no existing terminology data to be deleted, dicts=%v entries=%v rules=%v", dicts.deleted, entries.deleted, rules.deleted)
	}
	if len(dicts.items) != 2 || len(entries.items) != 2 || len(rules.items) != 2 {
		t.Fatalf("expected existing data counts preserved, dicts=%d entries=%d rules=%d", len(dicts.items), len(entries.items), len(rules.items))
	}
	if !seeds.seeded[terminologyLegacyNonMedicalCleanupKey] || !seeds.seeded[terminologyDefaultSceneCleanupKey] {
		t.Fatalf("expected deprecated cleanup states to be marked, got %#v", seeds.seeded)
	}
}

func TestEnsureSeedDataDoesNotAppendDefaultsToExistingDictionary(t *testing.T) {
	dicts := &termDictRepoStub{nextID: 1, items: map[uint64]*domain.TermDict{
		1: {ID: 1, Name: "影像报告", Domain: "医疗影像"},
	}}
	entries := &termEntryRepoStub{nextID: 1, items: map[uint64]domain.TermEntry{
		1: {ID: 1, DictID: 1, CorrectTerm: "用户自定义词"},
	}}
	rules := &termRuleRepoStub{nextID: 1, items: map[uint64]domain.CorrectionRule{
		1: {ID: 1, DictID: 1, MatchType: domain.RuleMatchRegex, Pattern: "用户规则", Replacement: "保留"},
	}}
	service := NewService(dicts, entries, rules, &termSeedRepoStub{})

	if err := service.EnsureSeedData(context.Background()); err != nil {
		t.Fatalf("EnsureSeedData returned error: %v", err)
	}

	if len(entries.items) != 1 || len(rules.items) != 1 {
		t.Fatalf("expected existing dictionary to remain untouched, entries=%d rules=%d", len(entries.items), len(rules.items))
	}
}

func TestEnsureSeedDataCreatesDefaultsForEmptyDatabase(t *testing.T) {
	dicts := &termDictRepoStub{}
	entries := &termEntryRepoStub{}
	rules := &termRuleRepoStub{}
	service := NewService(dicts, entries, rules, &termSeedRepoStub{})

	if err := service.EnsureSeedData(context.Background()); err != nil {
		t.Fatalf("EnsureSeedData returned error: %v", err)
	}

	if len(dicts.items) != 1 {
		t.Fatalf("expected one default dictionary, got %d", len(dicts.items))
	}
	if len(entries.items) == 0 {
		t.Fatal("expected default terminology entries")
	}
	if len(rules.items) == 0 {
		t.Fatal("expected default terminology rules")
	}
}
