package nlpengine

import (
	"context"
	"fmt"
	"testing"

	domain "github.com/lgt/asr/internal/domain/terminology"
)

type stubDictRepo struct {
	dicts map[uint64]*domain.TermDict
}

func (repo stubDictRepo) GetByID(_ context.Context, id uint64) (*domain.TermDict, error) {
	if dict, ok := repo.dicts[id]; ok {
		return dict, nil
	}
	return nil, fmt.Errorf("dict %d not found", id)
}

type stubEntryRepo struct {
	entries []domain.TermEntry
}

func (repo stubEntryRepo) ListByDict(context.Context, uint64) ([]domain.TermEntry, error) {
	return repo.entries, nil
}

type stubRuleRepo struct {
	rules []domain.CorrectionRule
}

func (repo stubRuleRepo) Create(context.Context, *domain.CorrectionRule) error { return nil }
func (repo stubRuleRepo) BatchCreate(context.Context, []domain.CorrectionRule) error {
	return nil
}
func (repo stubRuleRepo) GetByID(context.Context, uint64) (*domain.CorrectionRule, error) {
	return nil, nil
}
func (repo stubRuleRepo) ListByDict(context.Context, uint64) ([]domain.CorrectionRule, error) {
	return repo.rules, nil
}
func (repo stubRuleRepo) Update(context.Context, *domain.CorrectionRule) error { return nil }
func (repo stubRuleRepo) Delete(context.Context, uint64) error                 { return nil }
func (repo stubRuleRepo) DeleteByDict(context.Context, uint64) (int64, error)  { return 0, nil }

func TestCorrectorAppliesRulesBeforeTermEntries(t *testing.T) {
	dictID := uint64(1)
	corrector := NewCorrector(
		stubDictRepo{dicts: map[uint64]*domain.TermDict{dictID: {RuleProcessingEnabled: true, TextReplacementEnabled: true}}},
		stubEntryRepo{entries: []domain.TermEntry{{CorrectTerm: "舒张压", WrongVariants: []string{"舒张牙"}}}},
		stubRuleRepo{rules: []domain.CorrectionRule{
			{MatchType: domain.RuleMatchLiteral, Pattern: "舒张亚", Replacement: "舒张牙", Enabled: true},
		}},
	)

	result, corrections, err := corrector.Correct(context.Background(), &dictID, "患者舒张亚偏高")
	if err != nil {
		t.Fatalf("Correct returned error: %v", err)
	}
	if result != "患者舒张压偏高" {
		t.Fatalf("unexpected corrected text: %s", result)
	}
	if len(corrections["rules"]) != 1 || len(corrections["terms"]) != 1 {
		t.Fatalf("expected rule and term corrections, got %+v", corrections)
	}
}

func TestCorrectorRegexRule(t *testing.T) {
	dictID := uint64(1)
	corrector := NewCorrector(
		stubDictRepo{dicts: map[uint64]*domain.TermDict{dictID: {RuleProcessingEnabled: true, TextReplacementEnabled: true}}},
		stubEntryRepo{},
		stubRuleRepo{rules: []domain.CorrectionRule{{MatchType: domain.RuleMatchRegex, Pattern: `血压(\d+)/(\d+)`, Replacement: `血压$1-$2`, Enabled: true}}},
	)

	result, _, err := corrector.Correct(context.Background(), &dictID, "血压120/80")
	if err != nil {
		t.Fatalf("Correct returned error: %v", err)
	}
	if result != "血压120-80" {
		t.Fatalf("unexpected corrected text: %s", result)
	}
}

func TestCorrectorRegexReplacementKeepsAdjacentLiteralText(t *testing.T) {
	dictID := uint64(1)
	corrector := NewCorrector(
		stubDictRepo{dicts: map[uint64]*domain.TermDict{dictID: {RuleProcessingEnabled: true, TextReplacementEnabled: true}}},
		stubEntryRepo{},
		stubRuleRepo{rules: []domain.CorrectionRule{{MatchType: domain.RuleMatchRegex, Pattern: `(\d+)\s*毫米`, Replacement: `$1mm`, Enabled: true}}},
	)

	result, _, err := corrector.Correct(context.Background(), &dictID, "大小8毫米")
	if err != nil {
		t.Fatalf("Correct returned error: %v", err)
	}
	if result != "大小8mm" {
		t.Fatalf("unexpected corrected text: %s", result)
	}
}

func TestCorrectorNumberNormalizeRule(t *testing.T) {
	dictID := uint64(1)
	corrector := NewCorrector(
		stubDictRepo{dicts: map[uint64]*domain.TermDict{dictID: {RuleProcessingEnabled: true, TextReplacementEnabled: true}}},
		stubEntryRepo{},
		stubRuleRepo{rules: []domain.CorrectionRule{{MatchType: domain.RuleMatchNumberNormalize, Enabled: true}}},
	)

	result, _, err := corrector.Correct(context.Background(), &dictID, "大小十二乘二十三毫米，血钾二点三，第二点保持原样")
	if err != nil {
		t.Fatalf("Correct returned error: %v", err)
	}
	expected := "大小12x23mm，血钾2.3，第二点保持原样"
	if result != expected {
		t.Fatalf("unexpected corrected text: %s", result)
	}
}

func TestCorrectorSeparatesShangPhraseFromUpperLobe(t *testing.T) {
	dictID := uint64(1)
	corrector := NewCorrector(
		stubDictRepo{dicts: map[uint64]*domain.TermDict{dictID: {RuleProcessingEnabled: true, TextReplacementEnabled: true}}},
		stubEntryRepo{},
		stubRuleRepo{rules: []domain.CorrectionRule{
			{MatchType: domain.RuleMatchRegex, Pattern: `([左右])肺上[页业液]`, Replacement: `$1肺上叶`, Enabled: true},
			{MatchType: domain.RuleMatchRegex, Pattern: `(度|小|态|界|廓|限)上(可|清|对称)([\s，。；,;、]|$)`, Replacement: `$1尚$2$3`, Enabled: true},
		}},
	)

	result, _, err := corrector.Correct(context.Background(), &dictID, "左肺上页见结节，边界上清。")
	if err != nil {
		t.Fatalf("Correct returned error: %v", err)
	}
	expected := "左肺上叶见结节，边界尚清。"
	if result != expected {
		t.Fatalf("unexpected corrected text: %s", result)
	}
}

func TestCorrectorHallucinationTrimRemovesRepeatedClosingTail(t *testing.T) {
	dictID := uint64(1)
	corrector := NewCorrector(
		stubDictRepo{dicts: map[uint64]*domain.TermDict{dictID: {RuleProcessingEnabled: true, TextReplacementEnabled: true}}},
		stubEntryRepo{},
		stubRuleRepo{rules: []domain.CorrectionRule{{MatchType: domain.RuleMatchHallucinationTrim, Enabled: true}}},
	)

	closing := "较2024年5月13日片，左肾囊肿切除术后，术区少许渗出新见，请结合临床随诊复查。"
	input := "结论：左肾术后改变。" + closing + "总总总总。无意义长串继续转写。" + closing
	result, corrections, err := corrector.Correct(context.Background(), &dictID, input)
	if err != nil {
		t.Fatalf("Correct returned error: %v", err)
	}
	expected := "结论：左肾术后改变。" + closing
	if result != expected {
		t.Fatalf("unexpected corrected text: %s", result)
	}
	if len(corrections["rules"]) != 1 {
		t.Fatalf("expected one hallucination correction, got %+v", corrections)
	}
}

func TestCorrectorHallucinationTrimCollapsesRunawayClauseLoop(t *testing.T) {
	dictID := uint64(1)
	corrector := NewCorrector(
		stubDictRepo{dicts: map[uint64]*domain.TermDict{dictID: {RuleProcessingEnabled: true, TextReplacementEnabled: true}}},
		stubEntryRepo{},
		stubRuleRepo{rules: []domain.CorrectionRule{{MatchType: domain.RuleMatchHallucinationTrim, Enabled: true}}},
	)

	input := "开始。经260cm加硬导丝引导下置入右侧锁骨下动脉。复查造影见右侧锁骨下动脉管腔粗糙。" +
		"经260cm加硬导丝引导下置入右侧锁骨下动脉。复查造影见右侧锁骨下动脉管腔粗糙。" +
		"经260cm加硬导丝引导下置入右侧锁骨下动脉。复查造影见右侧锁骨下动脉管腔粗糙。" +
		"经260cm加硬导丝引导下置入右侧锁骨下动脉。复查造影见右侧锁骨下动脉管腔粗糙。结束。"
	result, _, err := corrector.Correct(context.Background(), &dictID, input)
	if err != nil {
		t.Fatalf("Correct returned error: %v", err)
	}
	expected := "开始。经260cm加硬导丝引导下置入右侧锁骨下动脉。复查造影见右侧锁骨下动脉管腔粗糙。结束。"
	if result != expected {
		t.Fatalf("unexpected corrected text: %s", result)
	}
}

func TestCorrectorSkipsDisabledRuleProcessing(t *testing.T) {
	dictID := uint64(1)
	corrector := NewCorrector(
		stubDictRepo{dicts: map[uint64]*domain.TermDict{dictID: {RuleProcessingEnabled: false, TextReplacementEnabled: true}}},
		stubEntryRepo{entries: []domain.TermEntry{{CorrectTerm: "舒张压", WrongVariants: []string{"舒张牙"}}}},
		stubRuleRepo{rules: []domain.CorrectionRule{{MatchType: domain.RuleMatchLiteral, Pattern: "舒张亚", Replacement: "舒张牙", Enabled: true}}},
	)

	result, corrections, err := corrector.Correct(context.Background(), &dictID, "患者舒张亚偏高")
	if err != nil {
		t.Fatalf("Correct returned error: %v", err)
	}
	if result != "患者舒张亚偏高" {
		t.Fatalf("unexpected corrected text: %s", result)
	}
	if len(corrections["rules"]) != 0 || len(corrections["terms"]) != 0 {
		t.Fatalf("expected no corrections, got %+v", corrections)
	}
}

func TestCorrectorSkipsDisabledTextReplacement(t *testing.T) {
	dictID := uint64(1)
	corrector := NewCorrector(
		stubDictRepo{dicts: map[uint64]*domain.TermDict{dictID: {RuleProcessingEnabled: true, TextReplacementEnabled: false}}},
		stubEntryRepo{entries: []domain.TermEntry{{CorrectTerm: "舒张压", WrongVariants: []string{"舒张牙"}}}},
		stubRuleRepo{rules: []domain.CorrectionRule{{MatchType: domain.RuleMatchLiteral, Pattern: "舒张亚", Replacement: "舒张牙", Enabled: true}}},
	)

	result, corrections, err := corrector.Correct(context.Background(), &dictID, "患者舒张亚偏高")
	if err != nil {
		t.Fatalf("Correct returned error: %v", err)
	}
	if result != "患者舒张牙偏高" {
		t.Fatalf("unexpected corrected text: %s", result)
	}
	if len(corrections["rules"]) != 1 || len(corrections["terms"]) != 0 {
		t.Fatalf("expected only rule corrections, got %+v", corrections)
	}
}
