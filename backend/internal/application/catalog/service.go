package catalog

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

// Service serves the read-only terminology catalog: a directory tree of
// markdown files, each holding a 9-column term table.
//
// Source resolution order:
//  1. If externalDir is configured AND points to a readable directory, files
//     are read from disk every request (no caching). This lets operators
//     update the markdown without rebuilding the binary.
//  2. Otherwise we fall back to the embedded snapshot under ./terms/, which
//     is shipped with the binary so the page works out of the box.
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
	terms []SectionTerm
}

// ErrFileNotFound is returned when a path cannot be resolved.
var ErrFileNotFound = errors.New("catalog file not found")

var catalogMenuMetadataFileNames = []string{
	"MENU.txt",
	"MENU.md",
	"menu.txt",
	"menu.md",
	"README.txt",
	"_menu.txt",
	"_menu.md",
}

// NewService builds a service. Pass "" to use only the embedded snapshot;
// pass an absolute or working-dir-relative path to load from disk instead.
func NewService(externalDir string) *Service {
	dir := strings.TrimSpace(externalDir)
	return &Service{externalDir: dir}
}

// DirectoryActive returns true when the on-disk catalog directory is being
// served (rather than the embedded fallback). Useful for diagnostics.
func (s *Service) DirectoryActive() bool {
	return s.externalDir != "" && dirReadable(s.externalDir)
}

// ActivePath returns the resolved source path (filesystem path or "<embedded>"
// when no external directory is available). Useful in log lines.
func (s *Service) ActivePath() string {
	if s.DirectoryActive() {
		return s.externalDir
	}
	return "<embedded>"
}

// Tree returns the catalog directory tree, each file annotated with parsed
// term counts (L1/L2/L3 + total). Directories are returned recursively.
func (s *Service) Tree() ([]TreeNode, error) {
	source, err := s.resolveSource()
	if err != nil {
		return nil, err
	}
	return buildTree(source, ".")
}

// GetFile returns the full markdown body + parsed term rows for a relative
// path. The path is normalised and must stay inside the catalog root.
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
	title, terms := parseMarkdownBody(clean, content)
	if title == "" {
		title = path.Base(clean)
	}
	return &FileDetail{
		Path:         clean,
		Name:         path.Base(clean),
		Title:        title,
		MarkdownBody: string(content),
		Terms:        terms,
	}, nil
}

// AllTerms enumerates every parsed term across every file. Used by the bulk
// Excel export.
func (s *Service) AllTerms() ([]SectionTerm, error) {
	return s.AllTermsInScope("")
}

// AllTermsInScope enumerates parsed terms under a directory or markdown file.
// An empty scope keeps the historical whole-catalog behaviour.
func (s *Service) AllTermsInScope(scope string) ([]SectionTerm, error) {
	cleanScope, err := safeRelScope(scope)
	if err != nil {
		return nil, err
	}
	source, err := s.resolveSource()
	if err != nil {
		return nil, err
	}
	var collected []SectionTerm
	matchedFile := false
	err = source.walk(func(relPath string, content []byte) error {
		if !pathWithinScope(relPath, cleanScope) {
			return nil
		}
		matchedFile = true
		_, terms := parseMarkdownBody(relPath, content)
		collected = append(collected, terms...)
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

// GenerateXLSX writes parsed terms to an xlsx workbook with the same columns
// the existing TermDict import endpoint accepts. This is used to build the
// checked-in per-department Excel files; downloads serve those files directly.
func (s *Service) GenerateXLSX(out *bytes.Buffer, scope ...string) (int, error) {
	scopePath := ""
	if len(scope) > 0 {
		scopePath = scope[0]
	}
	terms, err := s.AllTermsInScope(scopePath)
	if err != nil {
		return 0, err
	}

	wb := xlsxio.NewWorkbook("术语库")
	wb.AppendRow(
		"correct_term", "wrong_variants",
		"level", "english_or_abbr", "pinyin",
		"mixed_score", "rare_score", "glyph_score",
		"notes", "subsection_title", "source_path",
	)
	for _, term := range terms {
		wb.AppendRow(
			term.StandardTerm,
			strings.Join(term.CommonMisrecs, "|"),
			term.Level,
			term.EnglishOrAbbr,
			term.Pinyin,
			intToCell(term.MixedScore),
			intToCell(term.RareScore),
			intToCell(term.GlyphScore),
			term.Notes,
			term.SubsectionTitle,
			term.SourcePath,
		)
	}
	if err := wb.Encode(out); err != nil {
		return 0, err
	}
	return len(terms), nil
}

// ExportXLSX reads the built-in department Excel file from the catalog tree.
// The scope is usually a department directory such as "radiology".
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
			var buf bytes.Buffer
			count, err := s.GenerateXLSX(&buf, cleanScope)
			if err != nil {
				return "", nil, 0, err
			}
			filename := "term-catalog.xlsx"
			if cleanScope != "" {
				filename = path.Base(cleanScope) + ".xlsx"
			}
			return filename, buf.Bytes(), count, nil
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
	terms, err := s.AllTermsInScope(termScope)
	if err != nil {
		return "", nil, 0, err
	}
	return path.Base(excelPath), content, len(terms), nil
}

func intToCell(value int) string {
	if value == 0 {
		return "0"
	}
	return strings.TrimSpace(strings.TrimLeft(formatIntFast(value), " "))
}

// formatIntFast avoids importing strconv into this single tiny call site.
func formatIntFast(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
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

// AssertCatalogConsistent loads the active catalog source once and verifies
// the parser can read every file. Called from cmd/admin-api/main.go at
// startup so misconfigured directories surface immediately.
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

type diskSource struct {
	root string
}

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
	data, err := fs.ReadFile(termsFS, "terms/"+relPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrFileNotFound
	}
	return data, err
}

func (e *embedSource) walk(visit func(relPath string, content []byte) error) error {
	return fs.WalkDir(termsFS, "terms", func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !isMarkdownCatalogFile(p) || isCatalogMenuMetadataFile(p) {
			return nil
		}
		content, err := fs.ReadFile(termsFS, p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "terms/")
		return visit(rel, content)
	})
}

func (e *embedSource) list(dir string) ([]fs.DirEntry, error) {
	base := "terms"
	if dir != "." {
		base = "terms/" + dir
	}
	return fs.ReadDir(termsFS, base)
}

// ----- tree building --------------------------------------------------------

// buildTree walks the source directory and produces TreeNodes. Files have
// counts populated; directories use optional menu metadata files for labels.
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
		title, terms := parseMarkdownBody(relPath, content)
		if title == "" {
			title = strings.TrimSuffix(name, ".md")
		}
		node := TreeNode{
			Name:       name,
			Path:       relPath,
			IsDir:      false,
			Title:      title,
			TotalTerms: len(terms),
		}
		for _, term := range terms {
			switch term.Level {
			case "L1":
				node.L1Count++
			case "L2":
				node.L2Count++
			case "L3":
				node.L3Count++
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
	// Reject parent-directory escapes outright. We don't try to be clever with
	// filepath.Clean here because we want a forward-slash rel path.
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

func pathWithinScope(relPath string, scope string) bool {
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
			"menu_name:", "menu_name：",
			"menu:", "menu：",
			"title:", "title：",
			"name:", "name：",
			"菜单名称:", "菜单名称：",
			"菜单:", "菜单：",
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
