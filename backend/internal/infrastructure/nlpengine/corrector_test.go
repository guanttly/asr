package nlpengine

import (
	"context"
	"testing"

	domain "github.com/lgt/asr/internal/domain/terminology"
)

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

func TestCorrectorAppliesRulesBeforeTermEntries(t *testing.T) {
	dictID := uint64(1)
	corrector := NewCorrector(
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

func TestCorrectorNumberNormalizeRule(t *testing.T) {
	dictID := uint64(1)
	corrector := NewCorrector(
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
