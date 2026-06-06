package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	apprc "github.com/lgt/asr/internal/application/rulescatalog"
	domain "github.com/lgt/asr/internal/domain/terminology"
	"github.com/lgt/asr/internal/infrastructure/persistence"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
	"gorm.io/gorm"
)

const (
	maxRulesImportFileSize = 5 * 1024 * 1024
	maxRulesImportRows     = 5000
	rulesCatalogDictDomain = "radiology-rules"
	rulesCatalogDictName   = "影像通用规则"
)

// RulesCatalogHandler exposes read-only browsing of the rules catalog plus a
// batch-import endpoint that writes to correction_rules.
type RulesCatalogHandler struct {
	service  *apprc.Service
	dictRepo *persistence.DictRepo
	ruleRepo *persistence.RuleRepo
}

// NewRulesCatalogHandler builds a handler.
func NewRulesCatalogHandler(service *apprc.Service, dictRepo *persistence.DictRepo, ruleRepo *persistence.RuleRepo) *RulesCatalogHandler {
	return &RulesCatalogHandler{
		service:  service,
		dictRepo: dictRepo,
		ruleRepo: ruleRepo,
	}
}

// Register wires routes under the admin group.
func (h *RulesCatalogHandler) Register(group *gin.RouterGroup) {
	g := group.Group("/rules-catalog")
	g.GET("/tree", h.GetTree)
	g.GET("/file", h.GetFile)
	g.GET("/export.xlsx", h.ExportXLSX)
	g.POST("/import", h.BatchImport)
}

// GetTree returns the directory tree of the active rules catalog.
func (h *RulesCatalogHandler) GetTree(c *gin.Context) {
	tree, err := h.service.Tree()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, gin.H{
		"items":  tree,
		"source": h.service.ActivePath(),
	})
}

// GetFile streams parsed details for a single markdown file.
func (h *RulesCatalogHandler) GetFile(c *gin.Context) {
	pathParam := c.Query("path")
	detail, err := h.service.GetFile(pathParam)
	if err != nil {
		if errors.Is(err, apprc.ErrFileNotFound) {
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, "rules catalog file not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, detail)
}

// ExportXLSX returns the built-in workbook for a department directory.
func (h *RulesCatalogHandler) ExportXLSX(c *gin.Context) {
	pathParam := c.Query("path")
	filename, content, count, err := h.service.ExportXLSX(pathParam)
	if err != nil {
		if errors.Is(err, apprc.ErrFileNotFound) {
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, "rules catalog xlsx not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(filename)))
	c.Header("X-Rule-Count", fmt.Sprintf("%d", count))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content)
}

// BatchImport reads an xlsx file (multipart field "file") and upserts every
// enabled rule into correction_rules under the radiology-rules default dict.
// An optional query param dict_id overrides the default dict lookup.
func (h *RulesCatalogHandler) BatchImport(c *gin.Context) {
	ctx := c.Request.Context()

	// Resolve dict — honour explicit dict_id or fall back to default.
	var dictID uint64
	if rawID := c.Query("dict_id"); rawID != "" {
		id, err := strconv.ParseUint(rawID, 10, 64)
		if err != nil {
			response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid dict_id")
			return
		}
		dictID = id
	} else {
		dict, err := h.dictRepo.FindByDomain(ctx, rulesCatalogDictDomain)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				newDict := &domain.TermDict{Name: rulesCatalogDictName, Domain: rulesCatalogDictDomain, RuleProcessingEnabled: true, TextReplacementEnabled: true}
				if err := h.dictRepo.Create(ctx, newDict); err != nil {
					response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, "create default dict: "+err.Error())
					return
				}
				dictID = newDict.ID
			} else {
				response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
				return
			}
		} else {
			dictID = dict.ID
		}
	}

	// Parse the uploaded xlsx.
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "field 'file' required")
		return
	}
	if fileHeader.Size > maxRulesImportFileSize {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "file too large (max 5MB)")
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	defer f.Close()

	raw, err := io.ReadAll(io.LimitReader(f, maxRulesImportFileSize+1))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	rows, err := parseXLSXRows(raw)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "parse xlsx: "+err.Error())
		return
	}

	rules, err := ruleImportRowsToRules(rows)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	if err := validateRuleImportRules(rules); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	imported := 0
	for _, r := range rules {
		r.DictID = dictID
		if err := h.ruleRepo.Create(ctx, &r); err != nil {
			response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, fmt.Sprintf("insert rule %q: %v", r.Pattern, err))
			return
		}
		imported++
	}

	response.Success(c, gin.H{"imported": imported, "dict_id": dictID})
}

// ruleImportRowsToRules maps xlsx rows (with optional header) to domain rules.
// Accepted columns: pattern, replacement, match_type, priority, conflict_group,
// enabled, category, example, notes, subsection_title, source_path.
func ruleImportRowsToRules(rows [][]string) ([]domain.CorrectionRule, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("导入文件为空")
	}
	if len(rows) > maxRulesImportRows+1 {
		return nil, fmt.Errorf("单次导入最多支持 %d 条规则", maxRulesImportRows)
	}

	// Column index defaults (positional fallback).
	idxPattern := 0
	idxReplacement := 1
	idxMatchType := 2
	idxPriority := 3
	idxConflictGroup := 4
	idxEnabled := 5
	start := 0

	if looksLikeRuleImportHeader(rows[0]) {
		start = 1
		for i, cell := range rows[0] {
			switch strings.ToLower(strings.TrimSpace(cell)) {
			case "pattern", "错误模式":
				idxPattern = i
			case "replacement", "正确写法":
				idxReplacement = i
			case "match_type", "匹配方式":
				idxMatchType = i
			case "priority", "优先级":
				idxPriority = i
			case "conflict_group", "冲突组":
				idxConflictGroup = i
			case "enabled", "启用":
				idxEnabled = i
			}
		}
	}

	results := make([]domain.CorrectionRule, 0, len(rows)-start)
	for _, row := range rows[start:] {
		matchType := normaliseRuleMatchTypeStr(cellAt(row, idxMatchType))
		pattern := normalizeRuleImportPattern(cellAt(row, idxPattern), matchType)
		replacement := cellAt(row, idxReplacement)
		if pattern == "" && matchType != string(domain.RuleMatchNumberNormalize) && matchType != string(domain.RuleMatchHallucinationTrim) {
			continue
		}
		priority := parseIntCell(cellAt(row, idxPriority), 100)
		conflictGroup := cellAt(row, idxConflictGroup)
		enabled := parseEnabledStr(cellAt(row, idxEnabled))

		results = append(results, domain.CorrectionRule{
			Pattern:       pattern,
			Replacement:   replacement,
			MatchType:     domain.RuleMatchType(matchType),
			Priority:      priority,
			SortOrder:     priority,
			ConflictGroup: conflictGroup,
			Enabled:       enabled,
		})
	}
	return results, nil
}

func looksLikeRuleImportHeader(row []string) bool {
	for _, cell := range row {
		switch strings.ToLower(strings.TrimSpace(cell)) {
		case "pattern", "replacement", "错误模式", "正确写法", "match_type", "匹配方式":
			return true
		}
	}
	return false
}

func cellAt(row []string, idx int) string {
	if idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func normaliseRuleMatchTypeStr(v string) string {
	switch strings.ToLower(v) {
	case "regex", "regexp", "正则", "正则表达式", "高级匹配":
		return "regex"
	case "number_normalize", "number-normalize", "数值归一", "数字归一", "数字格式自动规范":
		return "number_normalize"
	case "hallucination_trim", "hallucination-trim", "幻觉裁剪", "幻觉去尾", "重复裁剪":
		return "hallucination_trim"
	}
	return "literal"
}

func normalizeRuleImportPattern(pattern string, matchType string) string {
	if matchType != string(domain.RuleMatchRegex) {
		return pattern
	}
	pattern = strings.ReplaceAll(pattern, `\|`, "|")
	pattern = strings.ReplaceAll(pattern, `\-`, "-")
	return pattern
}

func validateRuleImportRules(rules []domain.CorrectionRule) error {
	for i, rule := range rules {
		switch rule.MatchType {
		case domain.RuleMatchLiteral, domain.RuleMatchRegex, domain.RuleMatchNumberNormalize, domain.RuleMatchHallucinationTrim:
		default:
			return fmt.Errorf("第 %d 条规则的匹配方式无效: %s", i+1, rule.MatchType)
		}
		if rule.MatchType != domain.RuleMatchNumberNormalize && rule.MatchType != domain.RuleMatchHallucinationTrim && strings.TrimSpace(rule.Pattern) == "" {
			return fmt.Errorf("第 %d 条规则缺少错误模式", i+1)
		}
		if rule.MatchType == domain.RuleMatchRegex {
			if _, err := regexp.Compile(rule.Pattern); err != nil {
				return fmt.Errorf("第 %d 条规则的正则表达式无效: %s: %v", i+1, rule.Pattern, err)
			}
		}
	}
	return nil
}

func parseIntCell(v string, defaultVal int) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func parseEnabledStr(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "否", "no", "false", "0", "关闭", "off":
		return false
	}
	return true
}
