package voicecommand

import "context"

type DictRepository interface {
	Create(ctx context.Context, dict *Dict) error
	GetByID(ctx context.Context, id uint64) (*Dict, error)
	Update(ctx context.Context, dict *Dict) error
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context, offset, limit int) ([]*Dict, int64, error)
	ListByIDs(ctx context.Context, ids []uint64) ([]*Dict, error)
}

type EntryRepository interface {
	Create(ctx context.Context, entry *Entry) error
	GetByID(ctx context.Context, id uint64) (*Entry, error)
	ListByDict(ctx context.Context, dictID uint64) ([]Entry, error)
	ListByDicts(ctx context.Context, dictIDs []uint64) ([]Entry, error)
	Update(ctx context.Context, entry *Entry) error
	Delete(ctx context.Context, id uint64) error
}
