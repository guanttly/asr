package api

import "testing"

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
