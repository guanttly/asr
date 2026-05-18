package xlsxio

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestWorkbookRoundTrip(t *testing.T) {
	wb := NewWorkbook("术语词条")
	wb.AppendRow("correct_term", "wrong_variants")
	wb.AppendRow("胼胝体", "骈祉体|便支体")

	var buf bytes.Buffer
	if err := wb.Encode(&buf); err != nil {
		t.Fatal(err)
	}

	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip read: %v", err)
	}

	wantFiles := map[string]bool{
		"[Content_Types].xml":          false,
		"_rels/.rels":                  false,
		"xl/_rels/workbook.xml.rels":   false,
		"xl/workbook.xml":              false,
		"xl/worksheets/sheet1.xml":     false,
		"xl/sharedStrings.xml":         false,
	}
	for _, file := range reader.File {
		if _, ok := wantFiles[file.Name]; ok {
			wantFiles[file.Name] = true
		}
	}
	for name, found := range wantFiles {
		if !found {
			t.Errorf("missing required xlsx part: %s", name)
		}
	}

	sharedStrings := readZipFile(t, reader, "xl/sharedStrings.xml")
	if !strings.Contains(sharedStrings, "胼胝体") {
		t.Errorf("sharedStrings.xml missing CJK content: %s", sharedStrings)
	}
}

func readZipFile(t *testing.T, reader *zip.Reader, name string) string {
	t.Helper()
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	t.Fatalf("file %s not present in archive", name)
	return ""
}
