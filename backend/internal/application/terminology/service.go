package terminology

import (
	"context"

	domain "github.com/lgt/asr/internal/domain/terminology"
)

type seedDictionary struct {
	Name    string
	Domain  string
	Entries []domain.TermEntry
	Rules   []domain.CorrectionRule
}

// Service orchestrates terminology management use cases.
type Service struct {
	dictRepo  domain.DictRepository
	entryRepo domain.EntryRepository
	ruleRepo  domain.RuleRepository
}

// NewService creates a new terminology application service.
func NewService(
	dictRepo domain.DictRepository,
	entryRepo domain.EntryRepository,
	ruleRepo domain.RuleRepository,
) *Service {
	return &Service{
		dictRepo:  dictRepo,
		entryRepo: entryRepo,
		ruleRepo:  ruleRepo,
	}
}

// CreateDict creates a new terminology dictionary.
func (s *Service) CreateDict(ctx context.Context, req *CreateDictRequest) (*DictResponse, error) {
	dict := &domain.TermDict{
		Name:   req.Name,
		Domain: req.Domain,
	}
	if err := s.dictRepo.Create(ctx, dict); err != nil {
		return nil, err
	}
	return &DictResponse{ID: dict.ID, Name: dict.Name, Domain: dict.Domain}, nil
}

// ListDicts returns a paginated list of dictionaries.
func (s *Service) ListDicts(ctx context.Context, offset, limit int) ([]*DictResponse, int64, error) {
	dicts, total, err := s.dictRepo.List(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	items := make([]*DictResponse, len(dicts))
	for i, d := range dicts {
		items[i] = &DictResponse{ID: d.ID, Name: d.Name, Domain: d.Domain}
	}
	return items, total, nil
}

// GetDictEntries returns all entries of a dictionary.
func (s *Service) GetDictEntries(ctx context.Context, dictID uint64) ([]EntryResponse, error) {
	entries, err := s.entryRepo.ListByDict(ctx, dictID)
	if err != nil {
		return nil, err
	}
	items := make([]EntryResponse, len(entries))
	for i, e := range entries {
		items[i] = EntryResponse{
			ID:            e.ID,
			CorrectTerm:   e.CorrectTerm,
			WrongVariants: e.WrongVariants,
			Pinyin:        e.Pinyin,
		}
	}
	return items, nil
}

// CreateEntry creates a single term entry under a dictionary.
func (s *Service) CreateEntry(ctx context.Context, req *CreateEntryRequest) (*EntryResponse, error) {
	entry := domain.TermEntry{
		DictID:        req.DictID,
		CorrectTerm:   req.CorrectTerm,
		WrongVariants: req.WrongVariants,
	}
	if err := s.entryRepo.BatchCreate(ctx, []domain.TermEntry{entry}); err != nil {
		return nil, err
	}

	entries, err := s.entryRepo.ListByDict(ctx, req.DictID)
	if err != nil {
		return nil, err
	}

	for _, item := range entries {
		if item.CorrectTerm == req.CorrectTerm {
			return &EntryResponse{
				ID:            item.ID,
				CorrectTerm:   item.CorrectTerm,
				WrongVariants: item.WrongVariants,
				Pinyin:        item.Pinyin,
			}, nil
		}
	}

	return &EntryResponse{
		CorrectTerm:   req.CorrectTerm,
		WrongVariants: req.WrongVariants,
	}, nil
}

// GetDictRules returns all correction rules of a dictionary.
func (s *Service) GetDictRules(ctx context.Context, dictID uint64) ([]RuleResponse, error) {
	rules, err := s.ruleRepo.ListByDict(ctx, dictID)
	if err != nil {
		return nil, err
	}

	items := make([]RuleResponse, len(rules))
	for i, rule := range rules {
		items[i] = RuleResponse{
			ID:          rule.ID,
			Layer:       int(rule.Layer),
			Pattern:     rule.Pattern,
			Replacement: rule.Replacement,
			Enabled:     rule.Enabled,
		}
	}

	return items, nil
}

// CreateRule creates a correction rule under a dictionary.
func (s *Service) CreateRule(ctx context.Context, req *CreateRuleRequest) (*RuleResponse, error) {
	rule := &domain.CorrectionRule{
		DictID:      req.DictID,
		Layer:       domain.CorrectionLayer(req.Layer),
		Pattern:     req.Pattern,
		Replacement: req.Replacement,
		Enabled:     req.Enabled,
	}
	if err := s.ruleRepo.Create(ctx, rule); err != nil {
		return nil, err
	}

	return &RuleResponse{
		ID:          rule.ID,
		Layer:       int(rule.Layer),
		Pattern:     rule.Pattern,
		Replacement: rule.Replacement,
		Enabled:     rule.Enabled,
	}, nil
}

// EnsureSeedData creates a minimal default set of dictionaries, entries and rules.
func (s *Service) EnsureSeedData(ctx context.Context) error {
	seeds := []seedDictionary{
		{
			Name:   "医疗查房",
			Domain: "医疗",
			Entries: []domain.TermEntry{
				{CorrectTerm: "舒张压", WrongVariants: []string{"舒张亚", "舒张鸭"}, Pinyin: "shu zhang ya"},
				{CorrectTerm: "冠状动脉", WrongVariants: []string{"关状动脉", "冠状动漫"}, Pinyin: "guan zhuang dong mai"},
				{CorrectTerm: "心电图", WrongVariants: []string{"心电途", "心电土"}, Pinyin: "xin dian tu"},
			},
			Rules: []domain.CorrectionRule{
				{Layer: domain.LayerExactMatch, Pattern: "舒张亚", Replacement: "舒张压", Enabled: true},
				{Layer: domain.LayerEditDistance, Pattern: "关状动脉", Replacement: "冠状动脉", Enabled: true},
				{Layer: domain.LayerPinyinSimilar, Pattern: "心电途", Replacement: "心电图", Enabled: true},
			},
		},
		{
			Name:   "庭审记录",
			Domain: "法律",
			Entries: []domain.TermEntry{
				{CorrectTerm: "被告人", WrongVariants: []string{"被告银", "被告仍"}, Pinyin: "bei gao ren"},
				{CorrectTerm: "合议庭", WrongVariants: []string{"合一庭", "合议停"}, Pinyin: "he yi ting"},
			},
			Rules: []domain.CorrectionRule{
				{Layer: domain.LayerExactMatch, Pattern: "被告银", Replacement: "被告人", Enabled: true},
				{Layer: domain.LayerEditDistance, Pattern: "合一庭", Replacement: "合议庭", Enabled: true},
			},
		},
	}

	existing, _, err := s.dictRepo.List(ctx, 0, 1000)
	if err != nil {
		return err
	}

	existingByName := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		existingByName[item.Name] = struct{}{}
	}

	for _, seed := range seeds {
		if _, ok := existingByName[seed.Name]; ok {
			continue
		}

		dict := &domain.TermDict{Name: seed.Name, Domain: seed.Domain}
		if err := s.dictRepo.Create(ctx, dict); err != nil {
			return err
		}

		entries := make([]domain.TermEntry, len(seed.Entries))
		for i, entry := range seed.Entries {
			entry.DictID = dict.ID
			entries[i] = entry
		}
		if len(entries) > 0 {
			if err := s.entryRepo.BatchCreate(ctx, entries); err != nil {
				return err
			}
		}

		for _, rule := range seed.Rules {
			rule.DictID = dict.ID
			r := rule
			if err := s.ruleRepo.Create(ctx, &r); err != nil {
				return err
			}
		}
	}

	return nil
}

// BatchImport imports multiple term entries at once.
func (s *Service) BatchImport(ctx context.Context, req *BatchImportRequest) error {
	entries := make([]domain.TermEntry, len(req.Entries))
	for i, e := range req.Entries {
		entries[i] = domain.TermEntry{
			DictID:        req.DictID,
			CorrectTerm:   e.CorrectTerm,
			WrongVariants: e.WrongVariants,
		}
	}
	return s.entryRepo.BatchCreate(ctx, entries)
}
