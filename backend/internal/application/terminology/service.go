package terminology

import (
	"context"
	"fmt"
	"regexp"
	"strings"

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
	seedRepo  domain.SeedStateRepository
}

const (
	terminologySeedStateKey               = "terminology_seed_initialized_v1"
	terminologyDefaultRuleSeedStateKey    = "terminology_default_rules_seeded_v1"
	terminologyLegacyNonMedicalCleanupKey = "terminology_legacy_nonmedical_cleanup_v1"
	terminologyDefaultSceneCleanupKey     = "terminology_default_scene_cleanup_v1"
	terminologySeedDictionaryListLimit    = 1000
	maxBatchImportEntries                 = 5000
)

// NewService creates a new terminology application service.
func NewService(
	dictRepo domain.DictRepository,
	entryRepo domain.EntryRepository,
	ruleRepo domain.RuleRepository,
	seedRepo domain.SeedStateRepository,
) *Service {
	return &Service{
		dictRepo:  dictRepo,
		entryRepo: entryRepo,
		ruleRepo:  ruleRepo,
		seedRepo:  seedRepo,
	}
}

// CreateDict creates a new terminology dictionary.
func (s *Service) CreateDict(ctx context.Context, req *CreateDictRequest) (*DictResponse, error) {
	dict := &domain.TermDict{
		Name:                   req.Name,
		Domain:                 req.Domain,
		RuleProcessingEnabled:  boolValue(req.RuleProcessingEnabled, true),
		TextReplacementEnabled: boolValue(req.TextReplacementEnabled, true),
	}
	if err := s.dictRepo.Create(ctx, dict); err != nil {
		return nil, err
	}
	return newDictResponse(dict), nil
}

// UpdateDict updates a terminology dictionary.
func (s *Service) UpdateDict(ctx context.Context, id uint64, req *UpdateDictRequest) (*DictResponse, error) {
	dict, err := s.dictRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	dict.Name = req.Name
	dict.Domain = req.Domain
	if req.RuleProcessingEnabled != nil {
		dict.RuleProcessingEnabled = *req.RuleProcessingEnabled
	}
	if req.TextReplacementEnabled != nil {
		dict.TextReplacementEnabled = *req.TextReplacementEnabled
	}
	if err := s.dictRepo.Update(ctx, dict); err != nil {
		return nil, err
	}
	return newDictResponse(dict), nil
}

// DeleteDict deletes a terminology dictionary and its related entries and rules.
func (s *Service) DeleteDict(ctx context.Context, id uint64) error {
	if _, err := s.entryRepo.DeleteByDict(ctx, id); err != nil {
		return err
	}
	if _, err := s.ruleRepo.DeleteByDict(ctx, id); err != nil {
		return err
	}
	return s.dictRepo.Delete(ctx, id)
}

// ListDicts returns a paginated list of dictionaries.
func (s *Service) ListDicts(ctx context.Context, offset, limit int) ([]*DictResponse, int64, error) {
	dicts, total, err := s.dictRepo.List(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	items := make([]*DictResponse, len(dicts))
	for i, d := range dicts {
		items[i] = newDictResponse(d)
	}
	return items, total, nil
}

func newDictResponse(dict *domain.TermDict) *DictResponse {
	return &DictResponse{
		ID:                     dict.ID,
		Name:                   dict.Name,
		Domain:                 dict.Domain,
		RuleProcessingEnabled:  dict.RuleProcessingEnabled,
		TextReplacementEnabled: dict.TextReplacementEnabled,
	}
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
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
			}, nil
		}
	}

	return &EntryResponse{
		CorrectTerm:   req.CorrectTerm,
		WrongVariants: req.WrongVariants,
	}, nil
}

// UpdateEntry updates a term entry under a dictionary.
func (s *Service) UpdateEntry(ctx context.Context, req *UpdateEntryRequest) (*EntryResponse, error) {
	entry, err := s.entryRepo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	entry.DictID = req.DictID
	entry.CorrectTerm = req.CorrectTerm
	entry.WrongVariants = req.WrongVariants
	if err := s.entryRepo.Update(ctx, entry); err != nil {
		return nil, err
	}
	return &EntryResponse{
		ID:            entry.ID,
		CorrectTerm:   entry.CorrectTerm,
		WrongVariants: entry.WrongVariants,
	}, nil
}

// DeleteEntry deletes a term entry.
func (s *Service) DeleteEntry(ctx context.Context, id uint64) error {
	return s.entryRepo.Delete(ctx, id)
}

// ClearEntries removes every term entry from one dictionary.
func (s *Service) ClearEntries(ctx context.Context, dictID uint64) (int, error) {
	deleted, err := s.entryRepo.DeleteByDict(ctx, dictID)
	if err != nil {
		return 0, err
	}
	return int(deleted), nil
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
			ID:            rule.ID,
			MatchType:     string(rule.MatchType),
			Pattern:       rule.Pattern,
			Replacement:   rule.Replacement,
			Enabled:       rule.Enabled,
			SortOrder:     rule.SortOrder,
			Priority:      normalizeRulePriorityValue(rule.Priority, rule.SortOrder),
			ConflictGroup: rule.ConflictGroup,
		}
	}

	return items, nil
}

// CreateRule creates a correction rule under a dictionary.
func (s *Service) CreateRule(ctx context.Context, req *CreateRuleRequest) (*RuleResponse, error) {
	rule, err := normalizeRuleRequest(req.DictID, req.MatchType, req.Pattern, req.Replacement, req.Enabled, req.SortOrder, req.Priority, req.ConflictGroup)
	if err != nil {
		return nil, err
	}
	if err := s.ruleRepo.Create(ctx, rule); err != nil {
		return nil, err
	}

	return &RuleResponse{
		ID:            rule.ID,
		MatchType:     string(rule.MatchType),
		Pattern:       rule.Pattern,
		Replacement:   rule.Replacement,
		Enabled:       rule.Enabled,
		SortOrder:     rule.SortOrder,
		Priority:      normalizeRulePriorityValue(rule.Priority, rule.SortOrder),
		ConflictGroup: rule.ConflictGroup,
	}, nil
}

// BatchCreateRules creates multiple correction rules in one batch.
func (s *Service) BatchCreateRules(ctx context.Context, dictID uint64, rules []domain.CorrectionRule) (int, error) {
	if len(rules) == 0 {
		return 0, nil
	}
	pending := make([]domain.CorrectionRule, 0, len(rules))
	for _, rule := range rules {
		normalized, err := normalizeRuleRequest(dictID, string(rule.MatchType), rule.Pattern, rule.Replacement, rule.Enabled, rule.SortOrder, rule.Priority, rule.ConflictGroup)
		if err != nil {
			return 0, err
		}
		pending = append(pending, *normalized)
	}
	if err := s.ruleRepo.BatchCreate(ctx, pending); err != nil {
		return 0, err
	}
	return len(pending), nil
}

func normalizeRuleRequest(dictID uint64, matchType, pattern, replacement string, enabled bool, sortOrder, priority int, conflictGroup string) (*domain.CorrectionRule, error) {
	typeValue := domain.RuleMatchType(strings.TrimSpace(matchType))
	if typeValue == "" {
		typeValue = domain.RuleMatchLiteral
	}
	if typeValue != domain.RuleMatchLiteral && typeValue != domain.RuleMatchRegex && typeValue != domain.RuleMatchNumberNormalize {
		return nil, fmt.Errorf("invalid match_type: %s", matchType)
	}

	pattern = strings.TrimSpace(pattern)
	replacement = strings.TrimSpace(replacement)
	if typeValue != domain.RuleMatchNumberNormalize && pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}
	if typeValue == domain.RuleMatchRegex {
		if _, err := regexp.Compile(pattern); err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %w", err)
		}
	}
	if typeValue == domain.RuleMatchNumberNormalize {
		pattern = ""
		replacement = ""
	}
	if sortOrder <= 0 {
		sortOrder = 100
	}
	priority = normalizeRulePriorityValue(priority, sortOrder)

	rule := &domain.CorrectionRule{
		DictID:        dictID,
		MatchType:     typeValue,
		Pattern:       pattern,
		Replacement:   replacement,
		Enabled:       enabled,
		SortOrder:     sortOrder,
		Priority:      priority,
		ConflictGroup: strings.TrimSpace(conflictGroup),
	}
	return rule, nil
}

func normalizeRulePriorityValue(priority, sortOrder int) int {
	if priority > 0 {
		return priority
	}
	if sortOrder > 0 {
		return sortOrder
	}
	return 100
}

// UpdateRule updates a correction rule.
func (s *Service) UpdateRule(ctx context.Context, req *UpdateRuleRequest) (*RuleResponse, error) {
	rule, err := s.ruleRepo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	next, err := normalizeRuleRequest(req.DictID, req.MatchType, req.Pattern, req.Replacement, req.Enabled, req.SortOrder, req.Priority, req.ConflictGroup)
	if err != nil {
		return nil, err
	}
	rule.DictID = next.DictID
	rule.MatchType = next.MatchType
	rule.Pattern = next.Pattern
	rule.Replacement = next.Replacement
	rule.Enabled = next.Enabled
	rule.SortOrder = next.SortOrder
	rule.Priority = next.Priority
	rule.ConflictGroup = next.ConflictGroup
	if err := s.ruleRepo.Update(ctx, rule); err != nil {
		return nil, err
	}
	return &RuleResponse{
		ID:            rule.ID,
		MatchType:     string(rule.MatchType),
		Pattern:       rule.Pattern,
		Replacement:   rule.Replacement,
		Enabled:       rule.Enabled,
		SortOrder:     rule.SortOrder,
		Priority:      normalizeRulePriorityValue(rule.Priority, rule.SortOrder),
		ConflictGroup: rule.ConflictGroup,
	}, nil
}

// DeleteRule deletes a correction rule.
func (s *Service) DeleteRule(ctx context.Context, id uint64) error {
	return s.ruleRepo.Delete(ctx, id)
}

// ClearRules removes every correction rule from one dictionary.
func (s *Service) ClearRules(ctx context.Context, dictID uint64) (int, error) {
	deleted, err := s.ruleRepo.DeleteByDict(ctx, dictID)
	if err != nil {
		return 0, err
	}
	return int(deleted), nil
}

// EnsureSeedData creates default terminology dictionaries only for an empty
// installation. Existing dictionaries are always treated as user data.
func (s *Service) EnsureSeedData(ctx context.Context) error {
	if err := s.removeLegacyNonMedicalSeeds(ctx); err != nil {
		return err
	}
	if err := s.removeDeprecatedDefaultSceneSeeds(ctx); err != nil {
		return err
	}
	createdDefaults, err := s.ensureSeedDictionaries(ctx)
	if err != nil {
		return err
	}
	return s.ensureDefaultRules(ctx, createdDefaults)
}

// removeLegacyNonMedicalSeeds used to delete historical 庭审记录/会议纪要 seed
// dictionaries. It now only records the transition state; data deletion requires
// an explicit operator action.
func (s *Service) removeLegacyNonMedicalSeeds(ctx context.Context) error {
	cleaned, err := s.seedRepo.IsSeeded(ctx, terminologyLegacyNonMedicalCleanupKey)
	if err != nil {
		return err
	}
	if cleaned {
		return nil
	}

	return s.seedRepo.MarkSeeded(ctx, terminologyLegacyNonMedicalCleanupKey)
}

// removeDeprecatedDefaultSceneSeeds keeps old default dictionaries untouched.
// They may have been edited by users before this version, so startup must not
// remove them by display name.
func (s *Service) removeDeprecatedDefaultSceneSeeds(ctx context.Context) error {
	cleaned, err := s.seedRepo.IsSeeded(ctx, terminologyDefaultSceneCleanupKey)
	if err != nil {
		return err
	}
	if cleaned {
		return nil
	}

	return s.seedRepo.MarkSeeded(ctx, terminologyDefaultSceneCleanupKey)
}

func (s *Service) ensureSeedDictionaries(ctx context.Context) (bool, error) {
	seeded, err := s.seedRepo.IsSeeded(ctx, terminologySeedStateKey)
	if err != nil {
		return false, err
	}
	if seeded {
		return false, nil
	}

	existing, total, err := s.dictRepo.List(ctx, 0, 1)
	if err != nil {
		return false, err
	}
	if total > 0 || len(existing) > 0 {
		return false, s.seedRepo.MarkSeeded(ctx, terminologySeedStateKey)
	}

	for _, seed := range defaultTerminologySeeds() {
		dict := &domain.TermDict{Name: seed.Name, Domain: seed.Domain, RuleProcessingEnabled: true, TextReplacementEnabled: true}
		if err := s.dictRepo.Create(ctx, dict); err != nil {
			return false, err
		}

		entries := make([]domain.TermEntry, len(seed.Entries))
		for i, entry := range seed.Entries {
			entry.DictID = dict.ID
			entries[i] = entry
		}
		if len(entries) > 0 {
			if err := s.entryRepo.BatchCreate(ctx, entries); err != nil {
				return false, err
			}
		}
	}

	return true, s.seedRepo.MarkSeeded(ctx, terminologySeedStateKey)
}

func (s *Service) ensureDefaultRules(ctx context.Context, allowSeedMutation bool) error {
	seeded, err := s.seedRepo.IsSeeded(ctx, terminologyDefaultRuleSeedStateKey)
	if err != nil {
		return err
	}
	if seeded {
		return nil
	}
	if !allowSeedMutation {
		return s.seedRepo.MarkSeeded(ctx, terminologyDefaultRuleSeedStateKey)
	}

	dicts, _, err := s.dictRepo.List(ctx, 0, terminologySeedDictionaryListLimit)
	if err != nil {
		return err
	}
	dictByName := make(map[string]*domain.TermDict, len(dicts))
	for _, dict := range dicts {
		if _, exists := dictByName[dict.Name]; !exists {
			dictByName[dict.Name] = dict
		}
	}

	for _, seed := range defaultTerminologySeeds() {
		dict := dictByName[seed.Name]
		if dict == nil {
			dict = &domain.TermDict{Name: seed.Name, Domain: seed.Domain, RuleProcessingEnabled: true, TextReplacementEnabled: true}
			if err := s.dictRepo.Create(ctx, dict); err != nil {
				return err
			}
			dictByName[dict.Name] = dict
		}

		if err := s.ensureSeedEntries(ctx, dict.ID, seed.Entries); err != nil {
			return err
		}
		if err := s.ensureSeedRules(ctx, dict.ID, seed.Rules); err != nil {
			return err
		}
	}

	return s.seedRepo.MarkSeeded(ctx, terminologyDefaultRuleSeedStateKey)
}

func (s *Service) ensureSeedEntries(ctx context.Context, dictID uint64, entries []domain.TermEntry) error {
	if len(entries) == 0 {
		return nil
	}

	existing, err := s.entryRepo.ListByDict(ctx, dictID)
	if err != nil {
		return err
	}
	existingTerms := make(map[string]struct{}, len(existing))
	for _, entry := range existing {
		existingTerms[entry.CorrectTerm] = struct{}{}
	}

	pending := make([]domain.TermEntry, 0, len(entries))
	for _, entry := range entries {
		if _, exists := existingTerms[entry.CorrectTerm]; exists {
			continue
		}
		entry.DictID = dictID
		pending = append(pending, entry)
	}
	if len(pending) == 0 {
		return nil
	}
	return s.entryRepo.BatchCreate(ctx, pending)
}

func (s *Service) ensureSeedRules(ctx context.Context, dictID uint64, seedRules []domain.CorrectionRule) error {
	if len(seedRules) == 0 {
		return nil
	}
	rules, err := s.ruleRepo.ListByDict(ctx, dictID)
	if err != nil {
		return err
	}

	for _, seedRule := range seedRules {
		if seedRuleExists(rules, seedRule) {
			continue
		}
		seedRule.DictID = dictID
		rule := seedRule
		if err := s.ruleRepo.Create(ctx, &rule); err != nil {
			return err
		}
		rules = append(rules, rule)
	}
	return nil
}

func seedRuleExists(rules []domain.CorrectionRule, seedRule domain.CorrectionRule) bool {
	for _, rule := range rules {
		if rule.MatchType == seedRule.MatchType && rule.Pattern == seedRule.Pattern && rule.Replacement == seedRule.Replacement {
			return true
		}
	}
	return false
}

func defaultTerminologySeeds() []seedDictionary {
	return []seedDictionary{
		{
			Name:   "影像报告",
			Domain: "医疗影像",
			Entries: []domain.TermEntry{
				{CorrectTerm: "冠脉造影", WrongVariants: []string{"冠脉早影", "冠脉照影"}},
				{CorrectTerm: "左心室", WrongVariants: []string{"左新室", "左心事"}},
			},
			Rules: imagingReportRules(),
		},
	}
}

// commonMedicalRules are the shared medical post-processing rules every seed
// dictionary inherits: 口语数字归一化 + 生命体征结构化 + 医学单位/缩写大小写 + 罗马/希腊字符。
func commonMedicalRules() []domain.CorrectionRule {
	return []domain.CorrectionRule{
		{MatchType: domain.RuleMatchNumberNormalize, Enabled: true, SortOrder: 10},

		{MatchType: domain.RuleMatchRegex, Pattern: `血压\s*(\d{2,3})\s*[/／]\s*(\d{2,3})\s*(?:mmHg|毫米汞柱)?`, Replacement: `血压$1/$2mmHg`, Enabled: true, SortOrder: 20},
		{MatchType: domain.RuleMatchRegex, Pattern: `(心率|脉搏|脉率|呼吸|呼吸频率)\s*(\d{1,3})\s*次(?:每|/)?分(?:钟)?`, Replacement: `$1$2次/分`, Enabled: true, SortOrder: 21},
		{MatchType: domain.RuleMatchRegex, Pattern: `体温\s*(\d{2}(?:\.\d)?)\s*(?:度|℃|摄氏度)`, Replacement: `体温$1℃`, Enabled: true, SortOrder: 22},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?:血氧饱和度|SpO2|spo2|SPO2)\s*(\d{2,3})\s*%?`, Replacement: `SpO₂$1%`, Enabled: true, SortOrder: 23},

		{MatchType: domain.RuleMatchRegex, Pattern: `(\d{4})\s*年\s*(\d{1,2})\s*月\s*(\d{1,2})\s*[日号]`, Replacement: `$1-$2-$3`, Enabled: true, SortOrder: 30},
		{MatchType: domain.RuleMatchRegex, Pattern: `(\d{1,2})\s*月\s*(\d{1,2})\s*[日号]`, Replacement: `$1-$2`, Enabled: true, SortOrder: 31},

		{MatchType: domain.RuleMatchRegex, Pattern: `罗马\s*一`, Replacement: `Ⅰ`, Enabled: true, SortOrder: 40},
		{MatchType: domain.RuleMatchRegex, Pattern: `罗马\s*二`, Replacement: `Ⅱ`, Enabled: true, SortOrder: 41},
		{MatchType: domain.RuleMatchRegex, Pattern: `罗马\s*三`, Replacement: `Ⅲ`, Enabled: true, SortOrder: 42},
		{MatchType: domain.RuleMatchRegex, Pattern: `罗马\s*四`, Replacement: `Ⅳ`, Enabled: true, SortOrder: 43},
		{MatchType: domain.RuleMatchRegex, Pattern: `罗马\s*五`, Replacement: `Ⅴ`, Enabled: true, SortOrder: 44},
		{MatchType: domain.RuleMatchRegex, Pattern: `罗马\s*六`, Replacement: `Ⅵ`, Enabled: true, SortOrder: 45},

		{MatchType: domain.RuleMatchRegex, Pattern: `阿尔法`, Replacement: `α`, Enabled: true, SortOrder: 50},
		{MatchType: domain.RuleMatchRegex, Pattern: `贝塔`, Replacement: `β`, Enabled: true, SortOrder: 51},
		{MatchType: domain.RuleMatchRegex, Pattern: `伽马|伽玛|伽馬`, Replacement: `γ`, Enabled: true, SortOrder: 52},
		{MatchType: domain.RuleMatchRegex, Pattern: `德尔塔|德耳塔`, Replacement: `δ`, Enabled: true, SortOrder: 53},

		{MatchType: domain.RuleMatchRegex, Pattern: `(\d+(?:\.\d+)?)\s*微克`, Replacement: `$1μg`, Enabled: true, SortOrder: 54},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)(\d+(?:\.\d+)?)\s*umol\s*/?\s*L`, Replacement: `$1μmol/L`, Enabled: true, SortOrder: 55},

		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)(\d+(?:\.\d+)?)\s*mmhg`, Replacement: `$1mmHg`, Enabled: true, SortOrder: 60},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)(\d+(?:\.\d+)?)\s*mmol\s*/?\s*L`, Replacement: `$1mmol/L`, Enabled: true, SortOrder: 61},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)(\d+(?:\.\d+)?)\s*meq\s*/?\s*L`, Replacement: `$1mEq/L`, Enabled: true, SortOrder: 62},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)(\d+(?:\.\d+)?)\s*mg\s*/?\s*dL`, Replacement: `$1mg/dL`, Enabled: true, SortOrder: 63},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)(\d+(?:\.\d+)?)\s*mg\s*/?\s*L`, Replacement: `$1mg/L`, Enabled: true, SortOrder: 64},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)(\d+(?:\.\d+)?)\s*iu\s*/?\s*L`, Replacement: `$1IU/L`, Enabled: true, SortOrder: 65},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)(\d+(?:\.\d+)?)\s*g\s*/?\s*L`, Replacement: `$1g/L`, Enabled: true, SortOrder: 66},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)(\d+(?:\.\d+)?)\s*ml\b`, Replacement: `$1mL`, Enabled: true, SortOrder: 67},

		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)\bct\b`, Replacement: `CT`, Enabled: true, SortOrder: 70},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)\b(?:mr|mri|mra)\b`, Replacement: `MRI`, Enabled: true, SortOrder: 71},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)\b(?:ecg|ekg)\b`, Replacement: `ECG`, Enabled: true, SortOrder: 72},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)\b(?:copd|coad)\b`, Replacement: `COPD`, Enabled: true, SortOrder: 73},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)\bnstemi\b`, Replacement: `NSTEMI`, Enabled: true, SortOrder: 74},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)\bstemi\b`, Replacement: `STEMI`, Enabled: true, SortOrder: 75},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)\bpci\b`, Replacement: `PCI`, Enabled: true, SortOrder: 76},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)\bnyha\b`, Replacement: `NYHA`, Enabled: true, SortOrder: 77},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)\btnm\b`, Replacement: `TNM`, Enabled: true, SortOrder: 78},
		{MatchType: domain.RuleMatchRegex, Pattern: `(?i)\bhba1c\b`, Replacement: `HbA1c`, Enabled: true, SortOrder: 79},
	}
}

func medicalReportRules() []domain.CorrectionRule {
	return commonMedicalRules()
}

func imagingReportRules() []domain.CorrectionRule {
	rules := commonMedicalRules()
	rules = append(rules,
		domain.CorrectionRule{MatchType: domain.RuleMatchRegex, Pattern: `(\d+(?:\.\d+)?)\s*[xX*]\s*(\d+(?:\.\d+)?)\s*(mm|cm)`, Replacement: `$1×$2$3`, Enabled: true, SortOrder: 80},
		domain.CorrectionRule{MatchType: domain.RuleMatchRegex, Pattern: `(?i)(CT|MRI|MRA|DR|DSA|PET)\s+(\d+)`, Replacement: `$1$2`, Enabled: true, SortOrder: 81},
	)
	return rules
}

func labReportRules() []domain.CorrectionRule {
	rules := commonMedicalRules()
	rules = append(rules,
		domain.CorrectionRule{MatchType: domain.RuleMatchRegex, Pattern: `(血钾|血钠|血氯|血钙|血镁|血糖|肌酐|尿素|尿酸|白细胞|红细胞|血小板|血红蛋白|总胆固醇|甘油三酯)\s+([0-9]+(?:\.[0-9]+)?)`, Replacement: `$1$2`, Enabled: true, SortOrder: 82},
	)
	return rules
}

// BatchImport imports multiple term entries at once.
func (s *Service) BatchImport(ctx context.Context, req *BatchImportRequest) (*BatchImportResult, error) {
	if len(req.Entries) > maxBatchImportEntries {
		return nil, fmt.Errorf("单次导入最多支持 %d 条词条", maxBatchImportEntries)
	}

	existingEntries, err := s.entryRepo.ListByDict(ctx, req.DictID)
	if err != nil {
		return nil, err
	}
	existingTerms := map[string]struct{}{}
	for _, entry := range existingEntries {
		existingTerms[normalizeTermKey(entry.CorrectTerm)] = struct{}{}
	}

	entries := make([]domain.TermEntry, 0, len(req.Entries))
	skipped := 0
	seenInImport := map[string]struct{}{}
	for _, e := range req.Entries {
		correctTerm := strings.TrimSpace(e.CorrectTerm)
		if correctTerm == "" {
			skipped++
			continue
		}
		termKey := normalizeTermKey(correctTerm)
		if _, ok := existingTerms[termKey]; ok {
			skipped++
			continue
		}
		if _, ok := seenInImport[termKey]; ok {
			skipped++
			continue
		}
		seenInImport[termKey] = struct{}{}
		entries = append(entries, domain.TermEntry{
			DictID:        req.DictID,
			CorrectTerm:   correctTerm,
			WrongVariants: normalizeStringSlice(e.WrongVariants),
		})
	}
	if len(entries) == 0 {
		return &BatchImportResult{Imported: 0, Skipped: skipped}, nil
	}
	if err := s.entryRepo.BatchCreate(ctx, entries); err != nil {
		return nil, err
	}
	return &BatchImportResult{Imported: len(entries), Skipped: skipped}, nil
}

func normalizeTermKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeStringSlice(values []string) []string {
	items := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		items = append(items, trimmed)
	}
	return items
}
