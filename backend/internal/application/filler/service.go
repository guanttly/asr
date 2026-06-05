package filler

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	domain "github.com/lgt/asr/internal/domain/filler"
	termdomain "github.com/lgt/asr/internal/domain/terminology"
)

// dictNamePattern restricts filler dictionary names to Chinese characters,
// letters, digits, underscores, hyphens and spaces.
var dictNamePattern = regexp.MustCompile(`^[\p{Han}A-Za-z0-9_\- ]+$`)

// DictReferenceChecker reports how many workflow nodes still reference a
// filler dictionary, so deletion can be blocked while it is in use.
type DictReferenceChecker interface {
	CountFillerDictReferences(ctx context.Context, dictID uint64) (int, error)
}

type seedDictionary struct {
	Name        string
	Scene       string
	Description string
	IsBase      bool
	Entries     []domain.Entry
}

// Service orchestrates filler dictionary management.
type Service struct {
	dictRepo   domain.DictRepository
	entryRepo  domain.EntryRepository
	seedRepo   termdomain.SeedStateRepository
	refChecker DictReferenceChecker
}

const fillerSeedStateKey = "filler_seed_initialized_v1"

const fillerDictListLimit = 1000

func NewService(dictRepo domain.DictRepository, entryRepo domain.EntryRepository, seedRepo termdomain.SeedStateRepository) *Service {
	return &Service{dictRepo: dictRepo, entryRepo: entryRepo, seedRepo: seedRepo}
}

// SetReferenceChecker wires an optional checker used to block deletion of a
// dictionary that is still referenced by workflow nodes.
func (s *Service) SetReferenceChecker(checker DictReferenceChecker) {
	s.refChecker = checker
}

func validateDictName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: 词库名称不能为空", ErrFillerDictNameInvalid)
	}
	if !dictNamePattern.MatchString(name) {
		return fmt.Errorf("%w: 名称包含非法字符，仅支持中文、字母、数字、下划线和短横线", ErrFillerDictNameInvalid)
	}
	return nil
}

func (s *Service) CreateDict(ctx context.Context, req *CreateDictRequest) (*DictResponse, error) {
	name := strings.TrimSpace(req.Name)
	if err := validateDictName(name); err != nil {
		return nil, err
	}
	if err := s.validateBaseDictConstraint(ctx, 0, req.IsBase); err != nil {
		return nil, err
	}
	dict := &domain.Dict{
		Name: name, Scene: strings.TrimSpace(req.Scene), Description: strings.TrimSpace(req.Description), IsBase: req.IsBase,
	}
	if err := s.dictRepo.Create(ctx, dict); err != nil {
		return nil, err
	}
	return toDictResponse(dict), nil
}

func (s *Service) UpdateDict(ctx context.Context, id uint64, req *UpdateDictRequest) (*DictResponse, error) {
	dict, err := s.dictRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if dict == nil {
		return nil, fmt.Errorf("%w: %d", ErrFillerDictNotFound, id)
	}
	name := strings.TrimSpace(req.Name)
	if err := validateDictName(name); err != nil {
		return nil, err
	}
	if err := s.validateBaseDictConstraint(ctx, id, req.IsBase); err != nil {
		return nil, err
	}
	dict.Name = name
	dict.Scene = strings.TrimSpace(req.Scene)
	dict.Description = strings.TrimSpace(req.Description)
	dict.IsBase = req.IsBase
	if err := s.dictRepo.Update(ctx, dict); err != nil {
		return nil, err
	}
	return toDictResponse(dict), nil
}

func (s *Service) DeleteDict(ctx context.Context, id uint64) error {
	dict, err := s.dictRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if dict == nil {
		return fmt.Errorf("%w: %d", ErrFillerDictNotFound, id)
	}
	if dict.IsBase {
		return fmt.Errorf("%w: 基础语气词库不允许删除，请直接编辑内容", ErrFillerBaseDictProtected)
	}
	if s.refChecker != nil {
		count, err := s.refChecker.CountFillerDictReferences(ctx, id)
		if err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: 该语气词库仍被 %d 个工作流节点引用，请先解除引用后再删除", ErrFillerDictInUse, count)
		}
	}
	entries, err := s.entryRepo.ListByDict(ctx, id)
	if err != nil {
		return err
	}
	for i := range entries {
		if err := s.entryRepo.Delete(ctx, entries[i].ID); err != nil {
			return err
		}
	}
	return s.dictRepo.Delete(ctx, id)
}

func (s *Service) ListDicts(ctx context.Context, offset, limit int) ([]*DictResponse, int64, error) {
	dicts, total, err := s.dictRepo.List(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	items := make([]*DictResponse, len(dicts))
	for i, dict := range dicts {
		items[i] = toDictResponse(dict)
	}
	return items, total, nil
}

func (s *Service) GetDictEntries(ctx context.Context, dictID uint64) ([]EntryResponse, error) {
	dict, err := s.dictRepo.GetByID(ctx, dictID)
	if err != nil {
		return nil, err
	}
	if dict == nil {
		return nil, fmt.Errorf("%w: %d", ErrFillerDictNotFound, dictID)
	}
	entries, err := s.entryRepo.ListByDict(ctx, dictID)
	if err != nil {
		return nil, err
	}
	items := make([]EntryResponse, len(entries))
	for i, entry := range entries {
		items[i] = *toEntryResponse(&entry)
	}
	return items, nil
}

func (s *Service) CreateEntry(ctx context.Context, req *CreateEntryRequest) (*EntryResponse, error) {
	dict, err := s.dictRepo.GetByID(ctx, req.DictID)
	if err != nil {
		return nil, err
	}
	if dict == nil {
		return nil, fmt.Errorf("%w: %d", ErrFillerDictNotFound, req.DictID)
	}
	word := strings.TrimSpace(req.Word)
	if err := s.ensureWordUnique(ctx, req.DictID, word, 0); err != nil {
		return nil, err
	}
	entry := &domain.Entry{DictID: req.DictID, Word: word, Enabled: req.Enabled}
	if err := s.entryRepo.Create(ctx, entry); err != nil {
		return nil, err
	}
	return toEntryResponse(entry), nil
}

func (s *Service) UpdateEntry(ctx context.Context, req *UpdateEntryRequest) (*EntryResponse, error) {
	dict, err := s.dictRepo.GetByID(ctx, req.DictID)
	if err != nil {
		return nil, err
	}
	if dict == nil {
		return nil, fmt.Errorf("%w: %d", ErrFillerDictNotFound, req.DictID)
	}
	entry, err := s.entryRepo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("%w: %d", ErrFillerEntryNotFound, req.ID)
	}
	word := strings.TrimSpace(req.Word)
	if err := s.ensureWordUnique(ctx, req.DictID, word, req.ID); err != nil {
		return nil, err
	}
	entry.DictID = req.DictID
	entry.Word = word
	entry.Enabled = req.Enabled
	if err := s.entryRepo.Update(ctx, entry); err != nil {
		return nil, err
	}
	return toEntryResponse(entry), nil
}

// ensureWordUnique rejects a filler word that already exists in the same
// dictionary, ignoring the entry currently being edited (excludeID).
func (s *Service) ensureWordUnique(ctx context.Context, dictID uint64, word string, excludeID uint64) error {
	existing, err := s.entryRepo.ListByDict(ctx, dictID)
	if err != nil {
		return err
	}
	for i := range existing {
		if existing[i].ID == excludeID {
			continue
		}
		if strings.TrimSpace(existing[i].Word) == word {
			return fmt.Errorf("%w: 语气词「%s」已存在", ErrFillerEntryDuplicate, word)
		}
	}
	return nil
}

func (s *Service) DeleteEntry(ctx context.Context, id uint64) error {
	entry, err := s.entryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if entry == nil {
		return fmt.Errorf("%w: %d", ErrFillerEntryNotFound, id)
	}
	return s.entryRepo.Delete(ctx, id)
}

func (s *Service) EnsureSeedData(ctx context.Context) error {
	seeded, err := s.seedRepo.IsSeeded(ctx, fillerSeedStateKey)
	if err != nil {
		return err
	}
	if seeded {
		return nil
	}

	existing, total, err := s.dictRepo.List(ctx, 0, 1)
	if err != nil {
		return err
	}
	if total > 0 || len(existing) > 0 {
		return s.seedRepo.MarkSeeded(ctx, fillerSeedStateKey)
	}

	seeds := []seedDictionary{
		{
			Name:        "基础语气词库",
			Scene:       "通用",
			Description: "默认叠加到所有语气词过滤节点的基础口语词与停顿词库。",
			IsBase:      true,
			Entries: []domain.Entry{
				{Word: "嗯", Enabled: true},
				{Word: "啊", Enabled: true},
				{Word: "呃", Enabled: true},
				{Word: "那个", Enabled: true},
				{Word: "就是", Enabled: true},
				{Word: "然后", Enabled: true},
			},
		},
		{
			Name:        "直播口语场景",
			Scene:       "直播",
			Description: "适合直播、访谈等场景的额外口语词过滤。",
			IsBase:      false,
			Entries: []domain.Entry{
				{Word: "家人们", Enabled: true},
				{Word: "老铁", Enabled: true},
			},
		},
	}

	for _, seed := range seeds {
		dict := &domain.Dict{Name: seed.Name, Scene: seed.Scene, Description: seed.Description, IsBase: seed.IsBase}
		if err := s.dictRepo.Create(ctx, dict); err != nil {
			return err
		}
		for _, entry := range seed.Entries {
			item := entry
			item.DictID = dict.ID
			if err := s.entryRepo.Create(ctx, &item); err != nil {
				return err
			}
		}
	}

	return s.seedRepo.MarkSeeded(ctx, fillerSeedStateKey)
}

func (s *Service) validateBaseDictConstraint(ctx context.Context, currentID uint64, nextIsBase bool) error {
	if !nextIsBase {
		return nil
	}
	dicts, _, err := s.dictRepo.List(ctx, 0, fillerDictListLimit)
	if err != nil {
		return err
	}
	for _, dict := range dicts {
		if dict.IsBase && dict.ID != currentID {
			return fmt.Errorf("%w: 基础语气词库只能保留一个，请直接编辑现有基础库", ErrFillerBaseDictConflict)
		}
	}
	return nil
}

func toDictResponse(dict *domain.Dict) *DictResponse {
	return &DictResponse{ID: dict.ID, Name: dict.Name, Scene: dict.Scene, Description: dict.Description, IsBase: dict.IsBase}
}

func toEntryResponse(entry *domain.Entry) *EntryResponse {
	return &EntryResponse{ID: entry.ID, Word: entry.Word, Enabled: entry.Enabled}
}
