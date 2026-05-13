package api

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	appterm "github.com/lgt/asr/internal/application/terminology"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

const (
	maxTermImportFileSize = 5 * 1024 * 1024
	maxTermImportRows     = 5000
)

type xlsxCell struct {
	Reference string `xml:"r,attr"`
	Type      string `xml:"t,attr"`
	Value     string `xml:"v"`
	Inline    string `xml:"is>t"`
}

type xlsxRow struct {
	Cells []xlsxCell `xml:"c"`
}

type xlsxWorksheet struct {
	Rows []xlsxRow `xml:"sheetData>row"`
}

// TermHandler exposes terminology management endpoints.
type TermHandler struct {
	service *appterm.Service
}

// NewTermHandler creates a terminology handler.
func NewTermHandler(service *appterm.Service) *TermHandler {
	return &TermHandler{service: service}
}

// Register registers terminology routes.
func (h *TermHandler) Register(group *gin.RouterGroup) {
	group.GET("/term-dicts", h.ListDicts)
	group.POST("/term-dicts", h.CreateDict)
	group.PUT("/term-dicts/:id", h.UpdateDict)
	group.DELETE("/term-dicts/:id", h.DeleteDict)
	group.GET("/term-dicts/:id/entries", h.ListEntries)
	group.POST("/term-dicts/:id/entries", h.CreateEntry)
	group.PUT("/term-dicts/:id/entries/:entryId", h.UpdateEntry)
	group.DELETE("/term-dicts/:id/entries/:entryId", h.DeleteEntry)
	group.GET("/term-dicts/:id/rules", h.ListRules)
	group.POST("/term-dicts/:id/rules", h.CreateRule)
	group.PUT("/term-dicts/:id/rules/:ruleId", h.UpdateRule)
	group.DELETE("/term-dicts/:id/rules/:ruleId", h.DeleteRule)
	group.POST("/term-dicts/:id/import", h.BatchImport)
}

func (h *TermHandler) ListDicts(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	items, total, err := h.service.ListDicts(c.Request.Context(), offset, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, gin.H{"items": items, "total": total})
}

func (h *TermHandler) CreateDict(c *gin.Context) {
	var req appterm.CreateDictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	result, err := h.service.CreateDict(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *TermHandler) UpdateDict(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}

	var req appterm.UpdateDictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	result, err := h.service.UpdateDict(c.Request.Context(), dictID, &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *TermHandler) DeleteDict(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}

	if err := h.service.DeleteDict(c.Request.Context(), dictID); err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, gin.H{"deleted": true})
}

func (h *TermHandler) ListEntries(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}

	items, err := h.service.GetDictEntries(c.Request.Context(), dictID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, items)
}

func (h *TermHandler) CreateEntry(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}

	var req appterm.CreateEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	req.DictID = dictID

	result, err := h.service.CreateEntry(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *TermHandler) UpdateEntry(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	entryID, err := strconv.ParseUint(c.Param("entryId"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid entry id")
		return
	}

	var req appterm.UpdateEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	req.ID = entryID
	req.DictID = dictID

	result, err := h.service.UpdateEntry(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *TermHandler) DeleteEntry(c *gin.Context) {
	entryID, err := strconv.ParseUint(c.Param("entryId"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid entry id")
		return
	}

	if err := h.service.DeleteEntry(c.Request.Context(), entryID); err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, gin.H{"deleted": true})
}

func (h *TermHandler) ListRules(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}

	items, err := h.service.GetDictRules(c.Request.Context(), dictID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, items)
}

func (h *TermHandler) CreateRule(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}

	var req appterm.CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	req.DictID = dictID

	result, err := h.service.CreateRule(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *TermHandler) UpdateRule(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}
	ruleID, err := strconv.ParseUint(c.Param("ruleId"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid rule id")
		return
	}

	var req appterm.UpdateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	req.ID = ruleID
	req.DictID = dictID

	result, err := h.service.UpdateRule(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *TermHandler) DeleteRule(c *gin.Context) {
	ruleID, err := strconv.ParseUint(c.Param("ruleId"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid rule id")
		return
	}

	if err := h.service.DeleteRule(c.Request.Context(), ruleID); err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, gin.H{"deleted": true})
}

func (h *TermHandler) BatchImport(c *gin.Context) {
	dictID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict id")
		return
	}

	req, err := h.parseBatchImportRequest(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	req.DictID = dictID

	result, err := h.service.BatchImport(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	response.Success(c, result)
}

func (h *TermHandler) parseBatchImportRequest(c *gin.Context) (appterm.BatchImportRequest, error) {
	if fileHeader, err := c.FormFile("file"); err == nil {
		entries, err := parseTermEntryImportFile(fileHeader)
		if err != nil {
			return appterm.BatchImportRequest{}, err
		}
		return appterm.BatchImportRequest{Entries: entries}, nil
	}

	var req appterm.BatchImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return appterm.BatchImportRequest{}, err
	}
	if len(req.Entries) > maxTermImportRows {
		return appterm.BatchImportRequest{}, fmt.Errorf("单次导入最多支持 %d 条词条", maxTermImportRows)
	}
	return req, nil
}

func parseTermEntryImportFile(fileHeader *multipart.FileHeader) ([]appterm.CreateEntryRequest, error) {
	if fileHeader.Size > maxTermImportFileSize {
		return nil, fmt.Errorf("导入文件不能超过 5MB")
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".csv" && ext != ".tsv" && ext != ".txt" && ext != ".xlsx" {
		return nil, fmt.Errorf("仅支持 CSV/TSV/TXT/XLSX 文件导入")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if ext == ".xlsx" {
		content, err := io.ReadAll(io.LimitReader(file, maxTermImportFileSize+1))
		if err != nil {
			return nil, err
		}
		if len(content) > maxTermImportFileSize {
			return nil, fmt.Errorf("导入文件不能超过 5MB")
		}
		rows, err := parseXLSXRows(content)
		if err != nil {
			return nil, err
		}
		return termImportRowsToEntries(rows)
	}

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	if ext == ".tsv" || ext == ".txt" {
		reader.Comma = '\t'
	}
	rows, err := reader.ReadAll()
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("导入文件为空")
	}
	return termImportRowsToEntries(rows)
}

func termImportRowsToEntries(rows [][]string) ([]appterm.CreateEntryRequest, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("导入文件为空")
	}
	if len(rows) > maxTermImportRows+1 {
		return nil, fmt.Errorf("单次导入最多支持 %d 条词条", maxTermImportRows)
	}

	start := 0
	correctIndex := 0
	variantsIndex := 1
	if looksLikeTermImportHeader(rows[0]) {
		start = 1
		for i, cell := range rows[0] {
			switch strings.ToLower(strings.TrimSpace(cell)) {
			case "correct_term", "term", "标准词", "正确词", "术语":
				correctIndex = i
			case "wrong_variants", "variants", "aliases", "误写", "别名", "错误变体":
				variantsIndex = i
			}
		}
	}

	entries := make([]appterm.CreateEntryRequest, 0, len(rows)-start)
	for _, row := range rows[start:] {
		if correctIndex >= len(row) {
			entries = append(entries, appterm.CreateEntryRequest{})
			continue
		}
		entry := appterm.CreateEntryRequest{CorrectTerm: strings.TrimSpace(row[correctIndex])}
		if variantsIndex < len(row) {
			entry.WrongVariants = splitTermVariants(row[variantsIndex])
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func parseXLSXRows(content []byte) ([][]string, error) {
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("Excel 文件解析失败")
	}

	sharedStrings, err := readXLSXSharedStrings(archive)
	if err != nil {
		return nil, err
	}

	worksheet, err := openFirstXLSXWorksheet(archive)
	if err != nil {
		return nil, err
	}
	defer worksheet.Close()

	return readXLSXWorksheetRows(worksheet, sharedStrings)
}

func readXLSXSharedStrings(archive *zip.Reader) ([]string, error) {
	file := findZipFile(archive, "xl/sharedStrings.xml")
	if file == nil {
		return nil, nil
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	decoder := xml.NewDecoder(reader)
	items := []string{}
	var builder strings.Builder
	inSharedItem := false
	inText := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "si" {
				inSharedItem = true
				builder.Reset()
			}
			if inSharedItem && value.Name.Local == "t" {
				inText = true
			}
		case xml.CharData:
			if inSharedItem && inText {
				builder.Write([]byte(value))
			}
		case xml.EndElement:
			if value.Name.Local == "t" {
				inText = false
			}
			if value.Name.Local == "si" {
				items = append(items, builder.String())
				inSharedItem = false
			}
		}
	}
	return items, nil
}

func openFirstXLSXWorksheet(archive *zip.Reader) (io.ReadCloser, error) {
	if file := findZipFile(archive, "xl/worksheets/sheet1.xml"); file != nil {
		return file.Open()
	}
	for _, file := range archive.File {
		if strings.HasPrefix(file.Name, "xl/worksheets/") && strings.HasSuffix(file.Name, ".xml") {
			return file.Open()
		}
	}
	return nil, fmt.Errorf("Excel 文件没有可读取的工作表")
}

func findZipFile(archive *zip.Reader, name string) *zip.File {
	for _, file := range archive.File {
		if file.Name == name {
			return file
		}
	}
	return nil
}

func readXLSXWorksheetRows(reader io.Reader, sharedStrings []string) ([][]string, error) {
	var worksheet xlsxWorksheet
	if err := xml.NewDecoder(reader).Decode(&worksheet); err != nil {
		return nil, err
	}
	if len(worksheet.Rows) > maxTermImportRows+1 {
		return nil, fmt.Errorf("单次导入最多支持 %d 条词条", maxTermImportRows)
	}

	rows := make([][]string, 0, len(worksheet.Rows))
	for _, sourceRow := range worksheet.Rows {
		row := []string{}
		for cellPosition, cell := range sourceRow.Cells {
			columnIndex := xlsxColumnIndex(cell.Reference)
			if columnIndex < 0 {
				columnIndex = cellPosition
			}
			for len(row) <= columnIndex {
				row = append(row, "")
			}
			row[columnIndex] = xlsxCellText(cell, sharedStrings)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func xlsxCellText(cell xlsxCell, sharedStrings []string) string {
	if cell.Type == "inlineStr" {
		return strings.TrimSpace(cell.Inline)
	}
	if cell.Type == "s" {
		index, err := strconv.Atoi(strings.TrimSpace(cell.Value))
		if err == nil && index >= 0 && index < len(sharedStrings) {
			return strings.TrimSpace(sharedStrings[index])
		}
	}
	return strings.TrimSpace(cell.Value)
}

func xlsxColumnIndex(reference string) int {
	index := 0
	found := false
	for _, value := range reference {
		if value < 'A' || value > 'Z' {
			if value >= 'a' && value <= 'z' {
				value -= 'a' - 'A'
			} else {
				break
			}
		}
		found = true
		index = index*26 + int(value-'A'+1)
	}
	if !found {
		return -1
	}
	return index - 1
}

func looksLikeTermImportHeader(row []string) bool {
	for _, cell := range row {
		switch strings.ToLower(strings.TrimSpace(cell)) {
		case "correct_term", "term", "wrong_variants", "variants", "aliases", "标准词", "正确词", "术语", "误写", "别名", "错误变体":
			return true
		}
	}
	return false
}

func splitTermVariants(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '|' || r == ';' || r == '；' || r == '，' || r == ',' || r == '\n'
	})
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}
