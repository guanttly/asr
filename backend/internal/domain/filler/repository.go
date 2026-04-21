package filler

import "context"

// DictRepository defines persistence for filler dictionaries.
type DictRepository interface {
	Create(ctx context.Context, dict *Dict) error
	GetByID(ctx context.Context, id uint64) (*Dict, error)
	Update(ctx context.Context, dict *Dict) error
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context, offset, limit int) ([]*Dict, int64, error)
}

// EntryRepository manages filler word entries.
type EntryRepository interface {
	Create(ctx context.Context, entry *Entry) error
	GetByID(ctx context.Context, id uint64) (*Entry, error)
	ListByDict(ctx context.Context, dictID uint64) ([]Entry, error)
	ListAppliedByDict(ctx context.Context, dictID uint64) ([]Entry, error)
	Update(ctx context.Context, entry *Entry) error
	Delete(ctx context.Context, id uint64) error
}
