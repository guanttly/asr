package terminology

import "context"

// DictRepository defines persistence for terminology dictionaries.
type DictRepository interface {
	Create(ctx context.Context, dict *TermDict) error
	GetByID(ctx context.Context, id uint64) (*TermDict, error)
	Update(ctx context.Context, dict *TermDict) error
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context, offset, limit int) ([]*TermDict, int64, error)
}

// EntryRepository manages term entries within a dictionary.
type EntryRepository interface {
	BatchCreate(ctx context.Context, entries []TermEntry) error
	GetByID(ctx context.Context, id uint64) (*TermEntry, error)
	ListByDict(ctx context.Context, dictID uint64) ([]TermEntry, error)
	Update(ctx context.Context, entry *TermEntry) error
	Delete(ctx context.Context, id uint64) error
}

// RuleRepository manages correction rules.
type RuleRepository interface {
	Create(ctx context.Context, rule *CorrectionRule) error
	GetByID(ctx context.Context, id uint64) (*CorrectionRule, error)
	ListByDict(ctx context.Context, dictID uint64) ([]CorrectionRule, error)
	Update(ctx context.Context, rule *CorrectionRule) error
	Delete(ctx context.Context, id uint64) error
}

// SeedStateRepository tracks one-time initialization state for terminology seed data.
type SeedStateRepository interface {
	IsSeeded(ctx context.Context, key string) (bool, error)
	MarkSeeded(ctx context.Context, key string) error
}
