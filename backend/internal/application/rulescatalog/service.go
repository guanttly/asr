package rulescatalog

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/lgt/asr/pkg/xlsxio"
)

// Service serves the read-only rules catalog: a directory tree of markdown
// files, each holding a 9-column correction-rule table.
//
// Source resolution order:
//  1. If externalDir is configured AND points to a readable directory, files
//     are read from disk every request (no caching).
//  2. Otherwise we fall back to the embedded snapshot under ./rules/.
type Service struct {
	externalDir string

	once     sync.Once
	loadErr  error
	embedded map[string]parsedFile
	embedTop []string
}

type parsedFile struct {
	title string
	body  []byte
	rules []SectionRule
}

// ErrFileNotFound is returned when a path cannot be resolved.
var ErrFileNotFound = errors.New("rules catalog file not found")

var catalogMenuMetadataFileNames = []string{
	"MENU.txt", "MENU.md", "menu.txt", "menu.md",
	"README.txt", "_menu.txt", "_menu.md",
}

// NewService builds a service. Pass "" to use only the embedded snapshot.
func NewService(externalDir string) *Service {
	dir := strings.TrimSpace(externalDir)
	return &Service{externalDir: dir}
}

// DirectoryActive returns true when the on-disk directory is being served.
func (s *Service) DirectoryActive() bool {
	return s.externalDir != "" && dirReadable(s.externalDir)
}

// ActivePath returns the resolved source path or "<embedded>".
func (s *Service) ActivePath() string {
	if s.DirectoryActive() {
		return s.externalDir
	}
	return "<embedded>"
}

// Tree returns the catalog directory tree with rule counts.
func (s *Service) Tree() ([]TreeNode, error) {
	source, err := s.resolveSource()
	if err != nil {
		return nil, err
	}
	return buildTree(source, ".")
}

// GetFile returns markdown body + parsed rules for one file.
func (s *Service) GetFile(p string) (*FileDetail, error) {
	clean, err := safeRelPath(p)
	if err != nil {
		return nil, err
	}
	if isCatalogMenuMetadataFile(clean) {
		return nil, ErrFileNotFound
	}
	source, err := s.resolveSource()
	if err != nil {
		return nil, err
	}
	content, err := source.read(clean)
	if err != nil {
		return nil, err
	}
	title, rules := parseMarkdownBody(clean, content)
	if title == "" {
		title = path.Base(clean)
	}
	return &FileDetail{
		Path:         clean,
		Name:         path.Base(clean),
		Title:        title,
		MarkdownBody: string(content),
		Rules:        rules,
	}, nil
}

// AllRulesInScope aggregates rules across a directory or file.
func (s *Service) AllRulesInScope(scope string) ([]SectionRule, error) {
	cleanScope, err := safeRelScope(scope)
	if err != nil {
		return nil, err
	}
	source, err := s.resolveSource()
	if err != nil {
		return nil, err
	}
	var collected []SectionRule
	matchedFile := false
	err = source.walk(func(relPath string, content []byte) error {
		if !pathWithinScope(relPath, cleanScope) {
			return nil
		}
		matchedFile = true
		_, rules := parseMarkdownBody(relPath, content)
		collected = append(collected, rules...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if cleanScope != "" && !matchedFile {
		return nil, ErrFileNotFound
	}
	return collected, err
}

// GenerateXLSX writes parsed rules to an xlsx workbook in the format accepted
// by the rules batch-import endpoint.
func (s *Service) GenerateXLSX(out *bytes.Buffer, scope ...string) (int, error) {
	scopePath := ""
	if len(scope) > 0 {
		scopePath = scope[0]
	}
	rules, err := s.AllRulesInScope(scopePath)
	if err != nil {
		return 0, err
	}

	wb := xlsxio.NewWorkbook("规则库")
	wb.AppendRow(
		"pattern", "replacement", "match_type", "priority",
		"conflict_group", "enabled", "category", "example", "notes",
		"subsection_title", "source_path",
	)
	for _, rule := range rules {
		enabled := "是"
		if !rule.Enabled {
			enabled = "否"
		}
		wb.AppendRow(
			rule.Pattern,
			rule.Replacement,
			rule.MatchType,
			intToCell(rule.Priority),
			rule.ConflictGroup,
			enabled,
			rule.Category,
			rule.Example,
			rule.Notes,
			rule.SubsectionTitle,
			rule.SourcePath,
		)
	}
	if err := wb.Encode(out); err != nil {
		return 0, err
	}
	return len(rules), nil
}

// ExportXLSX reads the pre-built xlsx file from the catalog tree.
func (s *Service) ExportXLSX(scope string) (string, []byte, int, error) {
	cleanScope, err := safeRelScope(scope)
	if err != nil {
		return "", nil, 0, err
	}
	source, err := s.resolveSource()
	if err != nil {
		return "", nil, 0, err
	}
	excelPath := cleanScope
	if !isXLSXCatalogFile(excelPath) {
		found := findDirectoryExcelPath(source, cleanScope)
		if found == "" {
			return "", nil, 0, ErrFileNotFound
		}
		excelPath = found
	}
	content, err := source.read(excelPath)
	if err != nil {
		return "", nil, 0, err
	}
	termScope := cleanScope
	if isXLSXCatalogFile(termScope) {
		termScope = path.Dir(termScope)
		if termScope == "." {
			termScope = ""
		}
	}
	rules, err := s.AllRulesInScope(termScope)
	if err != nil {
		return "", nil, 0, err
	}
	return path.Base(excelPath), content, len(rules), nil
}

// AssertCatalogConsistent verifies that every file can be parsed.
func (s *Service) AssertCatalogConsistent() error {
	source, err := s.resolveSource()
	if err != nil {
		return err
	}
	return source.walk(func(relPath string, content []byte) error {
		_, _ = parseMarkdownBody(relPath, content)
		return nil
	})
}

// ----- source resolution ----------------------------------------------------

type catalogSource interface {
	read(relPath string) ([]byte, error)
	walk(visit func(relPath string, content []byte) error) error
	list(dir string) ([]fs.DirEntry, error)
}

func (s *Service) resolveSource() (catalogSource, error) {
	if s.DirectoryActive() {
		return &diskSource{root: s.externalDir}, nil
	}
	return &embedSource{}, nil
}

type diskSource struct{ root string }

func (d *diskSource) read(relPath string) ([]byte, error) {
	full := joinSafe(d.root, relPath)
	data, err := os.ReadFile(full)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrFileNotFound
	}
	return data, err
}

func (d *diskSource) walk(visit func(relPath string, content []byte) error) error {
	return fs.WalkDir(os.DirFS(d.root), ".", func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !isMarkdownCatalogFile(p) || isCatalogMenuMetadataFile(p) {
			return nil
		}
		content, err := os.ReadFile(joinSafe(d.root, p))
		if err != nil {
			return err
		}
		return visit(p, content)
	})
}

func (d *diskSource) list(dir string) ([]fs.DirEntry, error) {
	full := d.root
	if dir != "." {
		full = joinSafe(d.root, dir)
	}
	return os.ReadDir(full)
}

type embedSource struct{}

func (e *embedSource) read(relPath string) ([]byte, error) {
	data, err := fs.ReadFile(rulesFS, "rules/"+relPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrFileNotFound
	}
	return data, err
}

func (e *embedSource) walk(visit func(relPath string, content []byte) error) error {
	return fs.WalkDir(rulesFS, "rules", func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !isMarkdownCatalogFile(p) || isCatalogMenuMetadataFile(p) {
			return nil
		}
		content, err := fs.ReadFile(rulesFS, p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "rules/")
		return visit(rel, content)
	})
}

func (e *embedSource) list(dir string) ([]fs.DirEntry, error) {
	base := "rules"
	if dir != "." {
		base = "rules/" + dir
	}
	return fs.ReadDir(rulesFS, base)
}

// ----- tree building --------------------------------------------------------

func buildTree(source catalogSource, dir string) ([]TreeNode, error) {
	entries, err := source.list(dir)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	nodes := make([]TreeNode, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		relPath := name
		if dir != "." {
			relPath = dir + "/" + name
		}

		if entry.IsDir() {
			children, err := buildTree(source, relPath)
			if err != nil {
				return nil, err
			}
			if len(children) == 0 {
				continue
			}
			nodes = append(nodes, TreeNode{
				Name:      name,
				Path:      relPath,
				IsDir:     true,
				Title:     readDirectoryMenuTitle(source, relPath),
				ExcelPath: findDirectoryExcelPath(source, relPath),
				Children:  children,
			})
			continue
		}

		if isCatalogMenuMetadataFile(name) || isXLSXCatalogFile(name) || !isMarkdownCatalogFile(name) {
			continue
		}
		content, err := source.read(relPath)
		if err != nil {
			return nil, err
		}
		title, rules := parseMarkdownBody(relPath, content)
		if title == "" {
			title = strings.TrimSuffix(name, ".md")
		}
		node := TreeNode{
			Name:       name,
			Path:       relPath,
			IsDir:      false,
			Title:      title,
			TotalRules: len(rules),
		}
		for _, rule := range rules {
			switch rule.MatchType {
			case "regex":
				node.RegexCnt++
			case "literal":
				node.LiteralCnt++
			}
			if rule.Enabled {
				node.EnabledCnt++
			}
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// ----- helpers --------------------------------------------------------------

func safeRelPath(p string) (string, error) {
	clean := strings.TrimSpace(p)
	clean = strings.TrimPrefix(clean, "./")
	clean = strings.Trim(clean, "/")
	if clean == "" {
		return "", ErrFileNotFound
	}
	for _, segment := range strings.Split(clean, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", ErrFileNotFound
		}
	}
	if !strings.HasSuffix(strings.ToLower(clean), ".md") {
		return "", ErrFileNotFound
	}
	return clean, nil
}

func safeRelScope(p string) (string, error) {
	clean := strings.TrimSpace(p)
	clean = strings.TrimPrefix(clean, "./")
	clean = strings.Trim(clean, "/")
	if clean == "" {
		return "", nil
	}
	for _, segment := range strings.Split(clean, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", ErrFileNotFound
		}
	}
	if isCatalogMenuMetadataFile(clean) {
		return "", ErrFileNotFound
	}
	return clean, nil
}

func pathWithinScope(relPath, scope string) bool {
	if scope == "" {
		return true
	}
	return relPath == scope || strings.HasPrefix(relPath, scope+"/")
}

func isMarkdownCatalogFile(p string) bool {
	return strings.HasSuffix(strings.ToLower(path.Base(p)), ".md")
}

func isXLSXCatalogFile(p string) bool {
	base := strings.ToLower(path.Base(p))
	return strings.HasSuffix(base, ".xlsx") && !strings.HasPrefix(base, "~$")
}

func isCatalogMenuMetadataFile(p string) bool {
	base := strings.ToLower(path.Base(p))
	for _, name := range catalogMenuMetadataFileNames {
		if base == strings.ToLower(name) {
			return true
		}
	}
	return false
}

func readDirectoryMenuTitle(source catalogSource, dir string) string {
	for _, name := range catalogMenuMetadataFileNames {
		rel := name
		if dir != "." {
			rel = dir + "/" + name
		}
		content, err := source.read(rel)
		if err != nil {
			continue
		}
		if title := parseMenuTitle(content); title != "" {
			return title
		}
	}
	return ""
}

func findDirectoryExcelPath(source catalogSource, dir string) string {
	lookupDir := dir
	if lookupDir == "" {
		lookupDir = "."
	}
	entries, err := source.list(lookupDir)
	if err != nil {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	for _, entry := range entries {
		if entry.IsDir() || !isXLSXCatalogFile(entry.Name()) {
			continue
		}
		if lookupDir == "." {
			return entry.Name()
		}
		return lookupDir + "/" + entry.Name()
	}
	return ""
}

func parseMenuTitle(content []byte) string {
	for _, line := range strings.Split(string(content), "\n") {
		value := strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if value == "" {
			continue
		}
		value = strings.TrimSpace(strings.TrimLeft(value, "#"))
		for _, prefix := range []string{
			"menu_title:", "menu_title：",
			"title:", "title：",
			"菜单名称:", "菜单名称：",
			"名称:", "名称：",
		} {
			if strings.HasPrefix(strings.ToLower(value), prefix) {
				value = strings.TrimSpace(value[len(prefix):])
				break
			}
		}
		value = strings.Trim(value, " `\t\r\n\"'")
		if value != "" {
			return value
		}
	}
	return ""
}

func joinSafe(root, rel string) string {
	if rel == "" || rel == "." {
		return root
	}
	if strings.HasSuffix(root, string(os.PathSeparator)) {
		return root + rel
	}
	return root + string(os.PathSeparator) + strings.ReplaceAll(rel, "/", string(os.PathSeparator))
}

func dirReadable(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func intToCell(value int) string {
	if value == 0 {
		return "0"
	}
	const digits = "0123456789"
	negative := false
	if value < 0 {
		negative = true
		value = -value
	}
	out := [20]byte{}
	idx := len(out)
	for value > 0 {
		idx--
		out[idx] = digits[value%10]
		value /= 10
	}
	if negative {
		idx--
		out[idx] = '-'
	}
	return string(out[idx:])
}
