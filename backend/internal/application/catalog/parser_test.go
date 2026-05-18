package catalog

import (
	"bytes"
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
	count, err := svc.ExportXLSX(&buf)
	if err != nil {
		t.Fatalf("ExportXLSX: %v", err)
	}
	if count < 1000 {
		t.Errorf("ExportXLSX wrote %d rows, expected ≥ 1000", count)
	}
	if buf.Len() < 1000 {
		t.Errorf("workbook bytes = %d, looks suspiciously small", buf.Len())
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
