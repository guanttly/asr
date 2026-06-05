package filler

import (
	"context"
	"errors"
	"testing"

	domain "github.com/lgt/asr/internal/domain/filler"
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

type entryRepoStub struct{}

func (r *entryRepoStub) Create(_ context.Context, _ *domain.Entry) error            { return nil }
func (r *entryRepoStub) GetByID(_ context.Context, _ uint64) (*domain.Entry, error) { return nil, nil }
func (r *entryRepoStub) ListByDict(_ context.Context, _ uint64) ([]domain.Entry, error) {
	return nil, nil
}
func (r *entryRepoStub) ListAppliedByDict(_ context.Context, _ uint64) ([]domain.Entry, error) {
	return nil, nil
}
func (r *entryRepoStub) Update(_ context.Context, _ *domain.Entry) error { return nil }
func (r *entryRepoStub) Delete(_ context.Context, _ uint64) error        { return nil }

func TestCreateDictRejectsSecondBaseDict(t *testing.T) {
	repo := &dictRepoStub{items: map[uint64]*domain.Dict{
		1: {ID: 1, Name: "基础语气词库", IsBase: true},
	}}
	service := NewService(repo, &entryRepoStub{}, nil)

	_, err := service.CreateDict(context.Background(), &CreateDictRequest{Name: "新的基础库", Scene: "通用", IsBase: true})
	if err == nil {
		t.Fatal("expected error when creating second base dict")
	}
}

func TestDeleteDictRejectsBaseDict(t *testing.T) {
	repo := &dictRepoStub{items: map[uint64]*domain.Dict{
		1: {ID: 1, Name: "基础语气词库", IsBase: true},
	}}
	service := NewService(repo, &entryRepoStub{}, nil)

	err := service.DeleteDict(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error when deleting base dict")
	}
	if !errors.Is(err, ErrFillerBaseDictProtected) {
		t.Fatalf("expected ErrFillerBaseDictProtected, got %v", err)
	}
}

func TestGetDictEntriesReturnsNotFound(t *testing.T) {
	service := NewService(&dictRepoStub{items: map[uint64]*domain.Dict{}}, &entryRepoStub{}, nil)

	_, err := service.GetDictEntries(context.Background(), 99)
	if err == nil {
		t.Fatal("expected error when listing missing dict entries")
	}
	if !errors.Is(err, ErrFillerDictNotFound) {
		t.Fatalf("expected ErrFillerDictNotFound, got %v", err)
	}
}

func TestDeleteEntryReturnsNotFound(t *testing.T) {
	service := NewService(&dictRepoStub{}, &entryRepoStub{}, nil)

	err := service.DeleteEntry(context.Background(), 88)
	if err == nil {
		t.Fatal("expected error when deleting missing entry")
	}
	if !errors.Is(err, ErrFillerEntryNotFound) {
		t.Fatalf("expected ErrFillerEntryNotFound, got %v", err)
	}
}

type entryStoreStub struct {
	items  map[uint64]domain.Entry
	nextID uint64
}

func (r *entryStoreStub) Create(_ context.Context, entry *domain.Entry) error {
	if r.items == nil {
		r.items = map[uint64]domain.Entry{}
	}
	r.nextID++
	entry.ID = r.nextID
	r.items[entry.ID] = *entry
	return nil
}

func (r *entryStoreStub) GetByID(_ context.Context, id uint64) (*domain.Entry, error) {
	if item, ok := r.items[id]; ok {
		copyItem := item
		return &copyItem, nil
	}
	return nil, nil
}

func (r *entryStoreStub) ListByDict(_ context.Context, dictID uint64) ([]domain.Entry, error) {
	items := make([]domain.Entry, 0, len(r.items))
	for _, item := range r.items {
		if item.DictID == dictID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *entryStoreStub) ListAppliedByDict(_ context.Context, _ uint64) ([]domain.Entry, error) {
	return nil, nil
}
func (r *entryStoreStub) Update(_ context.Context, entry *domain.Entry) error {
	r.items[entry.ID] = *entry
	return nil
}
func (r *entryStoreStub) Delete(_ context.Context, id uint64) error {
	delete(r.items, id)
	return nil
}

type refCheckerStub struct {
	count int
}

func (r refCheckerStub) CountFillerDictReferences(_ context.Context, _ uint64) (int, error) {
	return r.count, nil
}

func TestCreateDictRejectsInvalidName(t *testing.T) {
	service := NewService(&dictRepoStub{items: map[uint64]*domain.Dict{}}, &entryRepoStub{}, nil)

	_, err := service.CreateDict(context.Background(), &CreateDictRequest{Name: "测试@#$", Scene: "通用"})
	if !errors.Is(err, ErrFillerDictNameInvalid) {
		t.Fatalf("expected ErrFillerDictNameInvalid, got %v", err)
	}
}

func TestCreateEntryRejectsDuplicate(t *testing.T) {
	dicts := &dictRepoStub{items: map[uint64]*domain.Dict{1: {ID: 1, Name: "场景库"}}}
	entries := &entryStoreStub{items: map[uint64]domain.Entry{
		1: {ID: 1, DictID: 1, Word: "嗯", Enabled: true},
	}, nextID: 1}
	service := NewService(dicts, entries, nil)

	_, err := service.CreateEntry(context.Background(), &CreateEntryRequest{DictID: 1, Word: " 嗯 ", Enabled: true})
	if !errors.Is(err, ErrFillerEntryDuplicate) {
		t.Fatalf("expected ErrFillerEntryDuplicate, got %v", err)
	}
}

func TestDeleteDictRejectsReferencedDict(t *testing.T) {
	dicts := &dictRepoStub{items: map[uint64]*domain.Dict{1: {ID: 1, Name: "场景库"}}}
	service := NewService(dicts, &entryStoreStub{}, nil)
	service.SetReferenceChecker(refCheckerStub{count: 2})

	err := service.DeleteDict(context.Background(), 1)
	if !errors.Is(err, ErrFillerDictInUse) {
		t.Fatalf("expected ErrFillerDictInUse, got %v", err)
	}
}
