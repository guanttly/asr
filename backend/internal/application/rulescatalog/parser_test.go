package rulescatalog

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedRulesCatalogParsesCleanly(t *testing.T) {
	svc := NewService("")
	tree, err := svc.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if len(tree) == 0 {
		t.Fatal("embedded rules catalog tree is empty")
	}
	if !tree[0].IsDir {
		t.Fatalf("embedded rules first node should be a directory, got %+v", tree[0])
	}

	rules, err := svc.AllRulesInScope("")
	if err != nil {
		t.Fatalf("AllRulesInScope: %v", err)
	}
	if len(rules) < 50 {
		t.Fatalf("total embedded rules = %d, expected ≥ 50", len(rules))
	}

	for _, rule := range rules {
		if rule.Pattern == "" {
			t.Errorf("rule in %s has empty pattern", rule.SourcePath)
			break
		}
		if rule.MatchType != "literal" && rule.MatchType != "regex" && rule.MatchType != "number_normalize" {
			t.Errorf("rule %q has invalid match_type %q", rule.Pattern, rule.MatchType)
			break
		}
	}
}

func TestGenerateXLSXProducesNonEmptyWorkbook(t *testing.T) {
	svc := NewService("")
	var buf bytes.Buffer
	count, err := svc.GenerateXLSX(&buf, "radiology")
	if err != nil {
		t.Fatalf("GenerateXLSX: %v", err)
	}
	if count < 50 {
		t.Errorf("GenerateXLSX wrote %d rows, expected ≥ 50", count)
	}
	if buf.Len() < 500 {
		t.Errorf("workbook bytes = %d, looks suspiciously small", buf.Len())
	}
}

func TestRulesCatalogTreeUsesDirectoryMenuMetadata(t *testing.T) {
	root := t.TempDir()
	deptDir := filepath.Join(root, "radiology")
	if err := os.Mkdir(deptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deptDir, "README.txt"), []byte("title: 影像规则\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	content := "# 通用规则\n\n## 单位\n\n" +
		"| 规则类型 | 错误模式 (pattern) | 正确写法 (replacement) | 匹配方式 | 优先级 | 冲突组 | 启用 | 示例 | 备注 |\n" +
		"| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n" +
		"| 单位归一 | 毫米 | mm | literal | 70 | unit-normalize | 是 | 5毫米→5mm | 测试 |\n"
	if err := os.WriteFile(filepath.Join(deptDir, "01-通用.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(root)
	tree, err := svc.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if len(tree) != 1 || !tree[0].IsDir {
		t.Fatalf("tree = %+v, want one directory node", tree)
	}
	if tree[0].Title != "影像规则" {
		t.Fatalf("directory title = %q, want 影像规则", tree[0].Title)
	}
	if len(tree[0].Children) != 1 {
		t.Fatalf("children count = %d, want 1", len(tree[0].Children))
	}
	if tree[0].Children[0].TotalRules != 1 {
		t.Fatalf("TotalRules = %d, want 1", tree[0].Children[0].TotalRules)
	}
}

func TestParseMarkdownBodyRulesTable(t *testing.T) {
	body := `# 规则测试

## 单位

| 规则类型 | 错误模式 (pattern) | 正确写法 (replacement) | 匹配方式 | 优先级 | 冲突组 | 启用 | 示例 | 备注 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 单位归一 | 毫米 | mm | literal | 70 | unit-normalize | 是 | 5毫米→5mm | 测试 |
| 乘号 | (\d)[xX乘]\s*(\d) | $1×$2 | regex | 80 | dimension | 否 | 12x13→12×13 |  |
`
	title, rules := parseMarkdownBody("test.md", []byte(body))
	if title != "规则测试" {
		t.Errorf("title = %q, want 规则测试", title)
	}
	if len(rules) != 2 {
		t.Fatalf("len(rules) = %d, want 2", len(rules))
	}
	if rules[0].Pattern != "毫米" || rules[0].Replacement != "mm" {
		t.Errorf("rule[0] = %+v", rules[0])
	}
	if rules[0].Enabled != true || rules[0].MatchType != "literal" {
		t.Errorf("rule[0] enabled/matchtype = %v/%q", rules[0].Enabled, rules[0].MatchType)
	}
	if rules[1].Enabled != false || rules[1].MatchType != "regex" {
		t.Errorf("rule[1] enabled/matchtype = %v/%q", rules[1].Enabled, rules[1].MatchType)
	}
}

func TestSafeRelPathRejection(t *testing.T) {
	cases := []string{"../etc/passwd", "", ".", "foo.txt"}
	for _, tc := range cases {
		if _, err := safeRelPath(tc); err == nil {
			t.Errorf("safeRelPath(%q) should fail", tc)
		}
	}
	if got, err := safeRelPath("radiology/01-通用.md"); err != nil || got != "radiology/01-通用.md" {
		t.Errorf("safeRelPath = (%q, %v)", got, err)
	}
}
