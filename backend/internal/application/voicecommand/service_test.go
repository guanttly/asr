package voicecommand

import (
	"context"
	"testing"

	termdomain "github.com/lgt/asr/internal/domain/terminology"
	domain "github.com/lgt/asr/internal/domain/voicecommand"
)

type dictRepoStub struct {
	items  map[uint64]*domain.Dict
	nextID uint64
}

func (r *dictRepoStub) Create(_ context.Context, dict *domain.Dict) error {
	if r.items == nil {
		r.items = map[uint64]*domain.Dict{}
	}
	if r.nextID == 0 {
		r.nextID = 1
	}
	dict.ID = r.nextID
	r.nextID += 1
	copyItem := *dict
	r.items[dict.ID] = &copyItem
	return nil
}

func (r *dictRepoStub) GetByID(_ context.Context, id uint64) (*domain.Dict, error) {
	item := r.items[id]
	if item == nil {
		return nil, nil
	}
	copyItem := *item
	return &copyItem, nil
}

func (r *dictRepoStub) Update(_ context.Context, dict *domain.Dict) error {
	copyItem := *dict
	r.items[dict.ID] = &copyItem
	return nil
}

func (r *dictRepoStub) Delete(_ context.Context, id uint64) error {
	delete(r.items, id)
	return nil
}

func (r *dictRepoStub) List(_ context.Context, _, _ int) ([]*domain.Dict, int64, error) {
	items := make([]*domain.Dict, 0, len(r.items))
	for _, item := range r.items {
		copyItem := *item
		items = append(items, &copyItem)
	}
	return items, int64(len(items)), nil
}

func (r *dictRepoStub) ListByIDs(_ context.Context, ids []uint64) ([]*domain.Dict, error) {
	items := make([]*domain.Dict, 0, len(ids))
	for _, id := range ids {
		if item, ok := r.items[id]; ok {
			copyItem := *item
			items = append(items, &copyItem)
		}
	}
	return items, nil
}

type entryRepoStub struct {
	items  map[uint64]*domain.Entry
	nextID uint64
}

func (r *entryRepoStub) Create(_ context.Context, entry *domain.Entry) error {
	if r.items == nil {
		r.items = map[uint64]*domain.Entry{}
	}
	if r.nextID == 0 {
		r.nextID = 1
	}
	entry.ID = r.nextID
	r.nextID += 1
	copyItem := *entry
	r.items[entry.ID] = &copyItem
	return nil
}

func (r *entryRepoStub) GetByID(_ context.Context, id uint64) (*domain.Entry, error) {
	item := r.items[id]
	if item == nil {
		return nil, nil
	}
	copyItem := *item
	return &copyItem, nil
}

func (r *entryRepoStub) ListByDict(_ context.Context, dictID uint64) ([]domain.Entry, error) {
	items := make([]domain.Entry, 0, len(r.items))
	for _, item := range r.items {
		if item.DictID != dictID {
			continue
		}
		items = append(items, *item)
	}
	return items, nil
}

func (r *entryRepoStub) ListByDicts(_ context.Context, dictIDs []uint64) ([]domain.Entry, error) {
	allowed := map[uint64]struct{}{}
	for _, id := range dictIDs {
		allowed[id] = struct{}{}
	}
	items := make([]domain.Entry, 0, len(r.items))
	for _, item := range r.items {
		if _, ok := allowed[item.DictID]; !ok {
			continue
		}
		items = append(items, *item)
	}
	return items, nil
}

func (r *entryRepoStub) Update(_ context.Context, entry *domain.Entry) error {
	copyItem := *entry
	r.items[entry.ID] = &copyItem
	return nil
}

func (r *entryRepoStub) Delete(_ context.Context, id uint64) error {
	delete(r.items, id)
	return nil
}

type seedRepoStub struct {
	seeded map[string]bool
}

func (r *seedRepoStub) IsSeeded(_ context.Context, key string) (bool, error) {
	return r.seeded[key], nil
}

func (r *seedRepoStub) MarkSeeded(_ context.Context, key string) error {
	if r.seeded == nil {
		r.seeded = map[string]bool{}
	}
	r.seeded[key] = true
	return nil
}

var _ termdomain.SeedStateRepository = (*seedRepoStub)(nil)

func TestCreateDictRejectsUnknownGroupKey(t *testing.T) {
	service := NewService(&dictRepoStub{}, &entryRepoStub{}, &seedRepoStub{})

	_, err := service.CreateDict(context.Background(), &CreateDictRequest{
		Name:     "非法分组",
		GroupKey: "free_style_key",
	})
	if err == nil {
		t.Fatal("expected invalid group key to be rejected")
	}
}

func TestCreateEntryRejectsUnknownIntentKey(t *testing.T) {
	dicts := &dictRepoStub{items: map[uint64]*domain.Dict{
		1: {
			ID:       1,
			Name:     "场景切换控制",
			GroupKey: domain.GroupKeySceneMode,
			IsBase:   true,
		},
	}}
	service := NewService(dicts, &entryRepoStub{}, &seedRepoStub{})

	_, err := service.CreateEntry(context.Background(), &CreateEntryRequest{
		DictID:     1,
		Intent:     "meeting",
		Label:      "会议模式",
		Utterances: []string{"会议模式"},
		Enabled:    true,
	})
	if err == nil {
		t.Fatal("expected legacy magic intent key to be rejected on create")
	}
}

func TestEnsureSeedDataUpgradesLegacyIntentKeys(t *testing.T) {
	dicts := &dictRepoStub{items: map[uint64]*domain.Dict{
		1: {
			ID:          1,
			Name:        "场景切换控制",
			GroupKey:    domain.GroupKeySceneMode,
			Description: "旧说明",
			IsBase:      true,
		},
	}}
	entries := &entryRepoStub{items: map[uint64]*domain.Entry{
		1: {
			ID:         1,
			DictID:     1,
			Intent:     domain.LegacyIntentReport,
			Label:      "报告模式",
			Utterances: []string{"切到报告模式"},
			Enabled:    true,
			SortOrder:  10,
		},
		2: {
			ID:         2,
			DictID:     1,
			Intent:     domain.LegacyIntentMeeting,
			Label:      "会议模式",
			Utterances: []string{"切到会议模式"},
			Enabled:    true,
			SortOrder:  20,
		},
	}}
	service := NewService(dicts, entries, &seedRepoStub{})

	if err := service.EnsureSeedData(context.Background()); err != nil {
		t.Fatalf("EnsureSeedData returned error: %v", err)
	}

	reportEntry, err := entries.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	meetingEntry, err := entries.GetByID(context.Background(), 2)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if reportEntry.Intent != domain.IntentKeySceneReportSwitch {
		t.Fatalf("expected legacy report intent upgraded, got %s", reportEntry.Intent)
	}
	if meetingEntry.Intent != domain.IntentKeySceneMeetingSwitch {
		t.Fatalf("expected legacy meeting intent upgraded, got %s", meetingEntry.Intent)
	}
	if len(reportEntry.Utterances) == 0 || len(meetingEntry.Utterances) == 0 {
		t.Fatal("expected builtin utterances preserved during upgrade")
	}
	if _, ok := dicts.items[1]; !ok {
		t.Fatal("expected builtin dict retained")
	}
	if dicts.items[1].Description == "旧说明" {
		t.Fatal("expected builtin dict description reconciled")
	}
	if !dicts.items[1].IsBase {
		t.Fatal("expected builtin dict to remain base group")
	}
	if reportEntry.Label == "" || meetingEntry.Label == "" {
		t.Fatal("expected upgraded intents to keep labels")
	}
}
