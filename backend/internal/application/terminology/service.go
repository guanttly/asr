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
	terminologySeedStateKey            = "terminology_seed_initialized_v1"
	terminologyDefaultRuleSeedStateKey = "terminology_default_rules_seeded_v1"
	terminologySeedDictionaryListLimit = 1000
	maxBatchImportEntries              = 5000
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
		Name:   req.Name,
		Domain: req.Domain,
	}
	if err := s.dictRepo.Create(ctx, dict); err != nil {
		return nil, err
	}
	return &DictResponse{ID: dict.ID, Name: dict.Name, Domain: dict.Domain}, nil
}

// UpdateDict updates a terminology dictionary.
func (s *Service) UpdateDict(ctx context.Context, id uint64, req *UpdateDictRequest) (*DictResponse, error) {
	dict, err := s.dictRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	dict.Name = req.Name
	dict.Domain = req.Domain
	if err := s.dictRepo.Update(ctx, dict); err != nil {
		return nil, err
	}
	return &DictResponse{ID: dict.ID, Name: dict.Name, Domain: dict.Domain}, nil
}

// DeleteDict deletes a terminology dictionary and its related entries and rules.
func (s *Service) DeleteDict(ctx context.Context, id uint64) error {
	entries, err := s.entryRepo.ListByDict(ctx, id)
	if err != nil {
		return err
	}
	for i := range entries {
		if err := s.entryRepo.Delete(ctx, entries[i].ID); err != nil {
			return err
		}
	}

	rules, err := s.ruleRepo.ListByDict(ctx, id)
	if err != nil {
		return err
	}
	for i := range rules {
		if err := s.ruleRepo.Delete(ctx, rules[i].ID); err != nil {
			return err
		}
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

// EnsureSeedData creates default terminology dictionaries and scene rules.
func (s *Service) EnsureSeedData(ctx context.Context) error {
	if err := s.ensureSeedDictionaries(ctx); err != nil {
		return err
	}
	return s.ensureDefaultRules(ctx)
}

func (s *Service) ensureSeedDictionaries(ctx context.Context) error {
	seeded, err := s.seedRepo.IsSeeded(ctx, terminologySeedStateKey)
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
		return s.seedRepo.MarkSeeded(ctx, terminologySeedStateKey)
	}

	for _, seed := range defaultTerminologySeeds() {
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
	}

	return s.seedRepo.MarkSeeded(ctx, terminologySeedStateKey)
}

func (s *Service) ensureDefaultRules(ctx context.Context) error {
	seeded, err := s.seedRepo.IsSeeded(ctx, terminologyDefaultRuleSeedStateKey)
	if err != nil {
		return err
	}
	if seeded {
		return nil
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
			dict = &domain.TermDict{Name: seed.Name, Domain: seed.Domain}
			if err := s.dictRepo.Create(ctx, dict); err != nil {
				return err
			}
			dictByName[dict.Name] = dict
		}

		if err := s.ensureSeedEntries(ctx, dict.ID, seed.Entries); err != nil {
			return err
		}
		if err := s.removeDeprecatedLiteralSeedRules(ctx, dict.ID); err != nil {
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

func (s *Service) removeDeprecatedLiteralSeedRules(ctx context.Context, dictID uint64) error {
	rules, err := s.ruleRepo.ListByDict(ctx, dictID)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.MatchType != domain.RuleMatchLiteral {
			continue
		}
		replacement := deprecatedSeedLiteralReplacement(rule.Pattern)
		if replacement == "" || replacement != rule.Replacement {
			continue
		}
		if err := s.ruleRepo.Delete(ctx, rule.ID); err != nil {
			return err
		}
	}
	return nil
}

func deprecatedSeedLiteralReplacement(pattern string) string {
	switch pattern {
	case "舒张亚":
		return "舒张压"
	case "关状动脉":
		return "冠状动脉"
	case "被告银":
		return "被告人"
	case "合一庭":
		return "合议庭"
	default:
		return ""
	}
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
			Name:   "写报告",
			Domain: "医疗",
			Entries: []domain.TermEntry{
				{CorrectTerm: "舒张压", WrongVariants: []string{"舒张亚", "舒张鸭"}},
				{CorrectTerm: "冠脉造影", WrongVariants: []string{"冠脉早影", "冠脉照影"}},
				{CorrectTerm: "阿司匹林", WrongVariants: []string{"阿斯匹林", "阿司匹灵"}},
			},
			Rules: medicalReportRules(),
		},
		{
			Name:   "医疗查房",
			Domain: "医疗",
			Entries: []domain.TermEntry{
				{CorrectTerm: "舒张压", WrongVariants: []string{"舒张亚", "舒张鸭"}},
				{CorrectTerm: "冠状动脉", WrongVariants: []string{"关状动脉", "冠状动漫"}},
				{CorrectTerm: "心电图", WrongVariants: []string{"心电途", "心电土"}},
			},
			Rules: medicalReportRules(),
		},
		{
			Name:   "影像报告",
			Domain: "医疗影像",
			Entries: []domain.TermEntry{
				{CorrectTerm: "冠脉造影", WrongVariants: []string{"冠脉早影", "冠脉照影"}},
				{CorrectTerm: "左心室", WrongVariants: []string{"左新室", "左心事"}},
			},
			Rules: imagingReportRules(),
		},
		{
			Name:   "检验报告",
			Domain: "医疗检验",
			Entries: []domain.TermEntry{
				{CorrectTerm: "血钾", WrongVariants: []string{"血甲", "血家"}},
				{CorrectTerm: "白细胞", WrongVariants: []string{"白西胞", "白细包"}},
			},
			Rules: labReportRules(),
		},
		{
			Name:   "庭审记录",
			Domain: "法律",
			Entries: []domain.TermEntry{
				{CorrectTerm: "被告人", WrongVariants: []string{"被告银", "被告仍"}},
				{CorrectTerm: "合议庭", WrongVariants: []string{"合一庭", "合议停"}},
			},
			Rules: legalRecordRules(),
		},
		{
			Name:   "会议纪要",
			Domain: "办公",
			Entries: []domain.TermEntry{
				{CorrectTerm: "会议纪要", WrongVariants: []string{"会议记要", "会议纪药"}},
				{CorrectTerm: "待办事项", WrongVariants: []string{"代办事项", "待办项目"}},
			},
			Rules: meetingMemoRules(),
		},
	}
}

func medicalReportRules() []domain.CorrectionRule {
	return []domain.CorrectionRule{
		{MatchType: domain.RuleMatchNumberNormalize, Enabled: true, SortOrder: 10},
		{MatchType: domain.RuleMatchRegex, Pattern: `血压\s*(\d{2,3})\s*[/／]\s*(\d{2,3})`, Replacement: `血压$1/$2mmHg`, Enabled: true, SortOrder: 40},
		{MatchType: domain.RuleMatchRegex, Pattern: `心率\s*(\d{2,3})\s*次(?:每|/)?分(?:钟)?`, Replacement: `心率$1次/分`, Enabled: true, SortOrder: 50},
	}
}

func imagingReportRules() []domain.CorrectionRule {
	return []domain.CorrectionRule{
		{MatchType: domain.RuleMatchNumberNormalize, Enabled: true, SortOrder: 10},
		{MatchType: domain.RuleMatchRegex, Pattern: `(\d+(?:\.\d+)?)x(\d+(?:\.\d+)?)(mm|cm)`, Replacement: `$1×$2$3`, Enabled: true, SortOrder: 40},
		{MatchType: domain.RuleMatchRegex, Pattern: `(CT|MR|MRI|DR)\s+(\d+)`, Replacement: `$1$2`, Enabled: true, SortOrder: 50},
	}
}

func labReportRules() []domain.CorrectionRule {
	return []domain.CorrectionRule{
		{MatchType: domain.RuleMatchNumberNormalize, Enabled: true, SortOrder: 10},
		{MatchType: domain.RuleMatchRegex, Pattern: `(血钾|血钠|血糖|肌酐|白细胞|红细胞|血红蛋白)\s+([0-9]+(?:\.[0-9]+)?)`, Replacement: `$1$2`, Enabled: true, SortOrder: 40},
		{MatchType: domain.RuleMatchRegex, Pattern: `([0-9]+(?:\.[0-9]+)?)\s+(mmol/L|g/L|mg/L|mg/dL)`, Replacement: `$1$2`, Enabled: true, SortOrder: 50},
	}
}

func legalRecordRules() []domain.CorrectionRule {
	return []domain.CorrectionRule{
		{MatchType: domain.RuleMatchNumberNormalize, Enabled: true, SortOrder: 10},
		{MatchType: domain.RuleMatchRegex, Pattern: `第\s*([0-9]+)\s*条`, Replacement: `第$1条`, Enabled: true, SortOrder: 40},
		{MatchType: domain.RuleMatchRegex, Pattern: `([（(][0-9]{4}[）)])\s*([^\s，,]{1,12})\s*([0-9]+)\s*号`, Replacement: `$1$2$3号`, Enabled: true, SortOrder: 50},
	}
}

func meetingMemoRules() []domain.CorrectionRule {
	return []domain.CorrectionRule{
		{MatchType: domain.RuleMatchNumberNormalize, Enabled: true, SortOrder: 10},
		{MatchType: domain.RuleMatchRegex, Pattern: `([0-2]?[0-9])点([0-5]?[0-9])分?`, Replacement: `$1:$2`, Enabled: true, SortOrder: 40},
		{MatchType: domain.RuleMatchRegex, Pattern: `(第[0-9一二三四五六七八九十]+[项条])\s+`, Replacement: `$1`, Enabled: true, SortOrder: 50},
	}
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
