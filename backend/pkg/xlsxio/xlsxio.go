// Package xlsxio writes minimal .xlsx workbooks with a single sheet.
//
// The project intentionally avoids heavy spreadsheet dependencies: the existing
// import path under interfaces/api already parses xlsx by hand using stdlib
// archive/zip + encoding/xml. This package mirrors that approach for the write
// side. It supports plain string cells only, which is all we need for term
// exports and templates.
package xlsxio

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Workbook is an in-progress xlsx file with a single sheet.
type Workbook struct {
	sheetName string
	rows      [][]string
}

// NewWorkbook starts an empty workbook with a named sheet.
func NewWorkbook(sheetName string) *Workbook {
	if strings.TrimSpace(sheetName) == "" {
		sheetName = "Sheet1"
	}
	return &Workbook{sheetName: sheetName}
}

// AppendRow appends a row of string cells.
func (w *Workbook) AppendRow(cells ...string) {
	row := make([]string, len(cells))
	copy(row, cells)
	w.rows = append(w.rows, row)
}

// Encode serialises the workbook into a zip-encoded xlsx file. The method is
// deliberately not named WriteTo because that signature is reserved by the
// io.WriterTo interface (which returns (int64, error)).
func (w *Workbook) Encode(out io.Writer) error {
	sharedStrings := make([]string, 0)
	sharedIndex := make(map[string]int)
	internSharedString := func(value string) int {
		if idx, ok := sharedIndex[value]; ok {
			return idx
		}
		idx := len(sharedStrings)
		sharedStrings = append(sharedStrings, value)
		sharedIndex[value] = idx
		return idx
	}

	var sheetData bytes.Buffer
	maxCols := 0
	totalStringRefs := 0
	for rowIdx, row := range w.rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
		fmt.Fprintf(&sheetData, `<row r="%d">`, rowIdx+1)
		for colIdx, value := range row {
			ref := cellReference(colIdx, rowIdx)
			if value == "" {
				fmt.Fprintf(&sheetData, `<c r="%s"/>`, ref)
				continue
			}
			idx := internSharedString(value)
			totalStringRefs++
			fmt.Fprintf(&sheetData, `<c r="%s" t="s"><v>%d</v></c>`, ref, idx)
		}
		sheetData.WriteString(`</row>`)
	}

	dimensionRef := "A1"
	if len(w.rows) > 0 && maxCols > 0 {
		dimensionRef = "A1:" + cellReference(maxCols-1, len(w.rows)-1)
	}

	var sheetBody bytes.Buffer
	sheetBody.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sheetBody.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	fmt.Fprintf(&sheetBody, `<dimension ref="%s"/>`, dimensionRef)
	sheetBody.WriteString(`<sheetData>`)
	sheetBody.Write(sheetData.Bytes())
	sheetBody.WriteString(`</sheetData></worksheet>`)

	var sharedStringsXML bytes.Buffer
	sharedStringsXML.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	fmt.Fprintf(&sharedStringsXML, `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">`, totalStringRefs, len(sharedStrings))
	for _, value := range sharedStrings {
		sharedStringsXML.WriteString(`<si><t xml:space="preserve">`)
		if err := xml.EscapeText(&sharedStringsXML, []byte(value)); err != nil {
			return err
		}
		sharedStringsXML.WriteString(`</t></si>`)
	}
	sharedStringsXML.WriteString(`</sst>`)

	writer := zip.NewWriter(out)
	files := []struct {
		Name string
		Body []byte
	}{
		{
			Name: "[Content_Types].xml",
			Body: []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>
</Types>`),
		},
		{
			Name: "_rels/.rels",
			Body: []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`),
		},
		{
			Name: "xl/_rels/workbook.xml.rels",
			Body: []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>
</Relationships>`),
		},
		{
			Name: "xl/workbook.xml",
			Body: []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets><sheet name="%s" sheetId="1" r:id="rId1"/></sheets>
</workbook>`, xmlEscapeAttribute(w.sheetName))),
		},
		{Name: "xl/worksheets/sheet1.xml", Body: sheetBody.Bytes()},
		{Name: "xl/sharedStrings.xml", Body: sharedStringsXML.Bytes()},
	}

	now := time.Now()
	for _, file := range files {
		header := &zip.FileHeader{Name: file.Name, Method: zip.Deflate, Modified: now}
		w, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := w.Write(file.Body); err != nil {
			return err
		}
	}
	return writer.Close()
}

func cellReference(col, row int) string {
	return columnLetters(col) + strconv.Itoa(row+1)
}

func columnLetters(col int) string {
	letters := ""
	col++
	for col > 0 {
		col--
		letters = string(rune('A'+(col%26))) + letters
		col /= 26
	}
	return letters
}

func xmlEscapeAttribute(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}
