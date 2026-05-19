package catalog

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedCatalogParsesCleanly(t *testing.T) {
	svc := NewService("")
	tree, err := svc.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if len(tree) == 0 {
		t.Fatal("embedded catalog tree is empty")
	}
	if !tree[0].IsDir || tree[0].ExcelPath == "" {
		t.Fatalf("embedded catalog first node = %+v, want department with ExcelPath", tree[0])
	}

	terms, err := svc.AllTerms()
	if err != nil {
		t.Fatalf("AllTerms: %v", err)
	}
	if len(terms) < 1000 {
		t.Fatalf("total embedded terms = %d, expected ≥ 1000", len(terms))
	}

	for _, term := range terms {
		if term.StandardTerm == "" {
			t.Errorf("term in %s missing standard term", term.SourcePath)
			break
		}
		if term.Level != "L1" && term.Level != "L2" && term.Level != "L3" {
			t.Errorf("term %s has invalid level %q", term.StandardTerm, term.Level)
			break
		}
	}
}

func TestSplitMisrecsTrimsAndDedup(t *testing.T) {
	result := splitMisrecs("骈祉体、便支体、片织体")
	want := []string{"骈祉体", "便支体", "片织体"}
	if len(result) != len(want) {
		t.Fatalf("len(result) = %d, want %d", len(result), len(want))
	}
	for i := range want {
		if result[i] != want[i] {
			t.Errorf("result[%d] = %q, want %q", i, result[i], want[i])
		}
	}
}

func TestSplitMisrecsRemovesArrowPrefix(t *testing.T) {
	result := splitMisrecs("胼胝体→骈祉体、便支体")
	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %v", result)
	}
	if result[0] != "骈祉体" || result[1] != "便支体" {
		t.Errorf("unexpected: %v", result)
	}
}

func TestExportXLSXContainsAllRows(t *testing.T) {
	svc := NewService("")
	var buf bytes.Buffer
	count, err := svc.GenerateXLSX(&buf, "radiology")
	if err != nil {
		t.Fatalf("GenerateXLSX: %v", err)
	}
	if count < 1000 {
		t.Errorf("GenerateXLSX wrote %d rows, expected ≥ 1000", count)
	}
	if buf.Len() < 1000 {
		t.Errorf("workbook bytes = %d, looks suspiciously small", buf.Len())
	}
}

func TestExportXLSXServesEmbeddedDepartmentWorkbook(t *testing.T) {
	svc := NewService("")
	filename, content, count, err := svc.ExportXLSX("radiology")
	if err != nil {
		t.Fatalf("ExportXLSX: %v", err)
	}
	if filename == "" {
		t.Fatal("filename is empty")
	}
	if count < 1000 {
		t.Errorf("ExportXLSX counted %d terms, expected ≥ 1000", count)
	}
	if len(content) < 1000 {
		t.Errorf("xlsx bytes = %d, looks suspiciously small", len(content))
	}
}

func TestCatalogTreeUsesDirectoryMenuMetadata(t *testing.T) {
	root := t.TempDir()
	deptDir := filepath.Join(root, "radiology")
	if err := os.Mkdir(deptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deptDir, "README.txt"), []byte("menu: 影像科\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deptDir, "01-通用.md"), []byte("# 01 · 通用\n\n| 标准术语 | 英文/缩写 | 拼音 | M | R | G | 等级 | 常见 ASR 误识别 | 备注 |\n| --- | --- | --- | ---: | ---: | ---: | --- | --- | --- |\n| CT | CT | ct | 1 | 0 | 0 | L1 | see tea | 测试 |\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(root)
	tree, err := svc.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if len(tree) != 1 {
		t.Fatalf("len(tree) = %d, want 1", len(tree))
	}
	if !tree[0].IsDir || tree[0].Title != "影像科" {
		t.Fatalf("department node = %+v, want title 影像科", tree[0])
	}
	if tree[0].ExcelPath != "" {
		t.Fatalf("ExcelPath = %q, want empty without an xlsx file", tree[0].ExcelPath)
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0].Name != "01-通用.md" {
		t.Fatalf("children = %+v, want only the markdown content file", tree[0].Children)
	}
}

func TestSafeRelPathRejectsEscape(t *testing.T) {
	cases := []string{
		"../etc/passwd",
		"foo/../../etc/passwd",
		"",
		".",
		"foo.txt",
	}
	for _, value := range cases {
		if _, err := safeRelPath(value); err == nil {
			t.Errorf("safeRelPath(%q) should fail but did not", value)
		}
	}
	if got, err := safeRelPath("section/01.md"); err != nil || got != "section/01.md" {
		t.Errorf("safeRelPath(section/01.md) = (%q,%v)", got, err)
	}
}
