package voicecommand

import (
	"context"
	"fmt"
	"strings"

	termdomain "github.com/lgt/asr/internal/domain/terminology"
	domain "github.com/lgt/asr/internal/domain/voicecommand"
)

type seedDictionary struct {
	Name        string
	GroupKey    string
	Description string
	IsBase      bool
	Entries     []domain.Entry
}

type Service struct {
	dictRepo  domain.DictRepository
	entryRepo domain.EntryRepository
	seedRepo  termdomain.SeedStateRepository
}

const voiceCommandSeedStateKey = "voice_command_seed_initialized_v1"
const voiceCommandDictListLimit = 1000

func NewService(dictRepo domain.DictRepository, entryRepo domain.EntryRepository, seedRepo termdomain.SeedStateRepository) *Service {
	return &Service{dictRepo: dictRepo, entryRepo: entryRepo, seedRepo: seedRepo}
}

func (s *Service) CreateDict(ctx context.Context, req *CreateDictRequest) (*DictResponse, error) {
	groupKey, err := normalizeBuiltinGroupKey(req.GroupKey)
	if err != nil {
		return nil, err
	}
	if err := s.validateBaseDictConstraint(ctx, 0, req.IsBase); err != nil {
		return nil, err
	}
	dict := &domain.Dict{
		Name: strings.TrimSpace(req.Name), GroupKey: groupKey, Description: strings.TrimSpace(req.Description), IsBase: req.IsBase,
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
		return nil, fmt.Errorf("%w: %d", ErrVoiceCommandDictNotFound, id)
	}
	groupKey, err := normalizeBuiltinGroupKey(req.GroupKey)
	if err != nil {
		return nil, err
	}
	if err := s.validateBaseDictConstraint(ctx, id, req.IsBase); err != nil {
		return nil, err
	}
	dict.Name = strings.TrimSpace(req.Name)
	dict.GroupKey = groupKey
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
		return fmt.Errorf("%w: %d", ErrVoiceCommandDictNotFound, id)
	}
	if dict.IsBase {
		return fmt.Errorf("%w: 基础控制指令组不允许删除，请直接编辑内容", ErrVoiceCommandBaseDictProtected)
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
		return nil, fmt.Errorf("%w: %d", ErrVoiceCommandDictNotFound, dictID)
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
		return nil, fmt.Errorf("%w: %d", ErrVoiceCommandDictNotFound, req.DictID)
	}
	intentKey, err := normalizeBuiltinIntentKey(dict.GroupKey, req.Intent)
	if err != nil {
		return nil, err
	}
	entry := &domain.Entry{
		DictID: req.DictID, Intent: intentKey, Label: strings.TrimSpace(req.Label), Utterances: normalizeUtterances(req.Utterances), Enabled: req.Enabled, SortOrder: req.SortOrder,
	}
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
		return nil, fmt.Errorf("%w: %d", ErrVoiceCommandDictNotFound, req.DictID)
	}
	intentKey, err := normalizeBuiltinIntentKey(dict.GroupKey, req.Intent)
	if err != nil {
		return nil, err
	}
	entry, err := s.entryRepo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("%w: %d", ErrVoiceCommandEntryNotFound, req.ID)
	}
	entry.DictID = req.DictID
	entry.Intent = intentKey
	entry.Label = strings.TrimSpace(req.Label)
	entry.Utterances = normalizeUtterances(req.Utterances)
	entry.Enabled = req.Enabled
	entry.SortOrder = req.SortOrder
	if err := s.entryRepo.Update(ctx, entry); err != nil {
		return nil, err
	}
	return toEntryResponse(entry), nil
}

func (s *Service) DeleteEntry(ctx context.Context, id uint64) error {
	entry, err := s.entryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if entry == nil {
		return fmt.Errorf("%w: %d", ErrVoiceCommandEntryNotFound, id)
	}
	return s.entryRepo.Delete(ctx, id)
}

func (s *Service) EnsureSeedData(ctx context.Context) error {
	if err := s.ensureBuiltinBaseGroups(ctx); err != nil {
		return err
	}
	if s.seedRepo == nil {
		return nil
	}
	seeded, err := s.seedRepo.IsSeeded(ctx, voiceCommandSeedStateKey)
	if err != nil {
		return err
	}
	if seeded {
		return nil
	}

	return s.seedRepo.MarkSeeded(ctx, voiceCommandSeedStateKey)
}

func (s *Service) ensureBuiltinBaseGroups(ctx context.Context) error {
	if s.dictRepo == nil || s.entryRepo == nil {
		return nil
	}
	dicts, _, err := s.dictRepo.List(ctx, 0, voiceCommandDictListLimit)
	if err != nil {
		return err
	}

	for _, group := range domain.BuiltinGroups() {
		dict, err := s.ensureBuiltinGroup(ctx, dicts, group)
		if err != nil {
			return err
		}
		if err := s.ensureBuiltinGroupEntries(ctx, dict.ID, group); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) ensureBuiltinGroup(ctx context.Context, dicts []*domain.Dict, group domain.BuiltinGroupSpec) (*domain.Dict, error) {
	for _, item := range dicts {
		if item == nil {
			continue
		}
		normalized, ok := domain.NormalizeGroupKey(item.GroupKey)
		if !ok || normalized != group.Key {
			continue
		}
		needsUpdate := item.Name != group.Name || item.GroupKey != group.Key || item.Description != group.Description || item.IsBase != group.IsBase
		if needsUpdate {
			item.Name = group.Name
			item.GroupKey = group.Key
			item.Description = group.Description
			item.IsBase = group.IsBase
			if err := s.dictRepo.Update(ctx, item); err != nil {
				return nil, err
			}
		}
		return item, nil
	}

	dict := &domain.Dict{
		Name:        group.Name,
		GroupKey:    group.Key,
		Description: group.Description,
		IsBase:      group.IsBase,
	}
	if err := s.dictRepo.Create(ctx, dict); err != nil {
		return nil, err
	}
	return dict, nil
}

func (s *Service) ensureBuiltinGroupEntries(ctx context.Context, dictID uint64, group domain.BuiltinGroupSpec) error {
	entries, err := s.entryRepo.ListByDict(ctx, dictID)
	if err != nil {
		return err
	}

	for _, spec := range group.Intents {
		var target *domain.Entry
		for i := range entries {
			normalized, ok := domain.NormalizeIntentKey(group.Key, entries[i].Intent)
			if !ok || normalized != spec.Key {
				continue
			}
			target = &entries[i]
			break
		}

		if target == nil {
			item := &domain.Entry{
				DictID:     dictID,
				Intent:     spec.Key,
				Label:      spec.DefaultLabel,
				Utterances: normalizeUtterances(spec.DefaultUtterances),
				Enabled:    true,
				SortOrder:  spec.SortOrder,
			}
			if err := s.entryRepo.Create(ctx, item); err != nil {
				return err
			}
			continue
		}

		mergedUtterances := normalizeUtterances(append(append([]string{}, spec.DefaultUtterances...), target.Utterances...))
		needsUpdate := target.Intent != spec.Key || len(mergedUtterances) != len(target.Utterances)
		if strings.TrimSpace(target.Label) == "" {
			target.Label = spec.DefaultLabel
			needsUpdate = true
		}
		if target.SortOrder == 0 {
			target.SortOrder = spec.SortOrder
			needsUpdate = true
		}
		if target.Intent != spec.Key {
			target.Intent = spec.Key
		}
		if len(mergedUtterances) != len(target.Utterances) {
			target.Utterances = mergedUtterances
		}
		if needsUpdate {
			if err := s.entryRepo.Update(ctx, target); err != nil {
				return err
			}
		}
	}

	return nil
}

func normalizeBuiltinGroupKey(groupKey string) (string, error) {
	trimmed := strings.TrimSpace(groupKey)
	group, ok := domain.BuiltinGroupByKey(trimmed)
	if !ok || group.Key != trimmed {
		return "", fmt.Errorf("unsupported voice command group key: %s", strings.TrimSpace(groupKey))
	}
	return group.Key, nil
}

func normalizeBuiltinIntentKey(groupKey string, intentKey string) (string, error) {
	canonicalGroupKey, ok := domain.NormalizeGroupKey(groupKey)
	if !ok {
		return "", fmt.Errorf("unsupported voice command group key: %s", strings.TrimSpace(groupKey))
	}
	trimmed := strings.TrimSpace(intentKey)
	intent, ok := domain.BuiltinIntentByKey(canonicalGroupKey, trimmed)
	if !ok || intent.Key != trimmed {
		return "", fmt.Errorf("unsupported voice command intent key %s for group %s", strings.TrimSpace(intentKey), canonicalGroupKey)
	}
	return intent.Key, nil
}

func (s *Service) validateBaseDictConstraint(ctx context.Context, currentID uint64, nextIsBase bool) error {
	if !nextIsBase {
		return nil
	}
	dicts, _, err := s.dictRepo.List(ctx, 0, voiceCommandDictListLimit)
	if err != nil {
		return err
	}
	for _, dict := range dicts {
		if dict.IsBase && dict.ID != currentID {
			return fmt.Errorf("%w: 基础控制指令组只能保留一个，请直接编辑现有基础组", ErrVoiceCommandBaseDictConflict)
		}
	}
	return nil
}

func normalizeUtterances(items []string) []string {
	result := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func toDictResponse(dict *domain.Dict) *DictResponse {
	return &DictResponse{ID: dict.ID, Name: dict.Name, GroupKey: dict.GroupKey, Description: dict.Description, IsBase: dict.IsBase}
}

func toEntryResponse(entry *domain.Entry) *EntryResponse {
	return &EntryResponse{ID: entry.ID, Intent: entry.Intent, Label: entry.Label, Utterances: entry.Utterances, Enabled: entry.Enabled, SortOrder: entry.SortOrder}
}
