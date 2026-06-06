package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRuleImportRowsNormalizesCatalogRegexEscapes(t *testing.T) {
	rows := [][]string{
		{"pattern", "replacement", "match_type", "priority", "conflict_group", "enabled"},
		{`(?i)stage\s*([Ⅰ-Ⅳ]\|[1-4])(a\|b\|c)?`, `Stage $1$2`, "regex", "50", "grading", "是"},
		{`(?i)ti[\-\s]?rads\s*([1-5])级?`, `TI-RADS $1 级`, "高级匹配", "50", "grading", "是"},
	}

	rules, err := ruleImportRowsToRules(rows)
	if err != nil {
		t.Fatalf("ruleImportRowsToRules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("len(rules) = %d, want 2", len(rules))
	}
	if rules[0].Pattern != `(?i)stage\s*([Ⅰ-Ⅳ]|[1-4])(a|b|c)?` {
		t.Fatalf("rules[0].Pattern = %q", rules[0].Pattern)
	}
	if rules[1].Pattern != `(?i)ti[-\s]?rads\s*([1-5])级?` {
		t.Fatalf("rules[1].Pattern = %q", rules[1].Pattern)
	}
	if rules[1].MatchType != "regex" {
		t.Fatalf("rules[1].MatchType = %q, want regex", rules[1].MatchType)
	}
	if err := validateRuleImportRules(rules); err != nil {
		t.Fatalf("validateRuleImportRules: %v", err)
	}
}

func TestValidateRuleImportRulesRejectsInvalidRegexBeforeInsert(t *testing.T) {
	rows := [][]string{
		{"pattern", "replacement", "match_type", "priority", "conflict_group", "enabled"},
		{"舒张亚", "舒张压", "literal", "100", "", "是"},
		{"(?i)[", "", "regex", "50", "", "是"},
	}

	rules, err := ruleImportRowsToRules(rows)
	if err != nil {
		t.Fatalf("ruleImportRowsToRules: %v", err)
	}
	if err := validateRuleImportRules(rules); err == nil {
		t.Fatal("validateRuleImportRules should reject invalid regex")
	}
}

func TestRuleImportRowsKeepsHallucinationTrimWithoutPattern(t *testing.T) {
	rows := [][]string{
		{"pattern", "replacement", "match_type", "priority", "conflict_group", "enabled"},
		{"", "", "hallucination_trim", "29", "hallucination", "是"},
	}

	rules, err := ruleImportRowsToRules(rows)
	if err != nil {
		t.Fatalf("ruleImportRowsToRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("len(rules) = %d, want 1", len(rules))
	}
	if rules[0].MatchType != "hallucination_trim" {
		t.Fatalf("rules[0].MatchType = %q, want hallucination_trim", rules[0].MatchType)
	}
	if err := validateRuleImportRules(rules); err != nil {
		t.Fatalf("validateRuleImportRules: %v", err)
	}
}

func TestDownloadRulesImportTemplateRoundTripsThroughParser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	NewTermHandler(nil).DownloadRulesImportTemplate(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	rows, err := parseXLSXRows(recorder.Body.Bytes())
	if err != nil {
		t.Fatalf("parseXLSXRows: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("template rows = %d, want at least 2", len(rows))
	}
	wantHeader := []string{"pattern", "replacement", "match_type", "priority", "conflict_group", "enabled", "category", "example", "notes", "subsection_title", "source_path"}
	if len(rows[0]) != len(wantHeader) {
		t.Fatalf("header len = %d, want %d: %#v", len(rows[0]), len(wantHeader), rows[0])
	}
	for i, want := range wantHeader {
		if rows[0][i] != want {
			t.Fatalf("header[%d] = %q, want %q", i, rows[0][i], want)
		}
	}

	rules, err := ruleImportRowsToRules(rows)
	if err != nil {
		t.Fatalf("ruleImportRowsToRules: %v", err)
	}
	if len(rules) != 4 {
		t.Fatalf("len(rules) = %d, want 4", len(rules))
	}
	if err := validateRuleImportRules(rules); err != nil {
		t.Fatalf("validateRuleImportRules: %v", err)
	}
}
