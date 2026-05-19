package catalog

// TreeNode is one entry in the catalog directory tree. A node is either a
// directory (with Children) or a file (with Path pointing at the markdown).
type TreeNode struct {
	Name       string     `json:"name"` // display name (file or dir)
	Path       string     `json:"path"` // forward-slash path relative to the catalog root
	IsDir      bool       `json:"is_dir"`
	Title      string     `json:"title,omitempty"` // files: parsed H1; dirs: menu metadata title
	ExcelPath  string     `json:"excel_path,omitempty"`
	TotalTerms int        `json:"total_terms,omitempty"`
	L1Count    int        `json:"l1_count,omitempty"`
	L2Count    int        `json:"l2_count,omitempty"`
	L3Count    int        `json:"l3_count,omitempty"`
	Children   []TreeNode `json:"children,omitempty"`
}

// FileDetail bundles the raw markdown body with the parsed term rows for a
// single file. The frontend renders MarkdownBody for the prose; Terms is also
// returned so it can be exported individually if we ever want to.
type FileDetail struct {
	Path         string        `json:"path"`
	Name         string        `json:"name"`
	Title        string        `json:"title"`
	MarkdownBody string        `json:"markdown_body"`
	Terms        []SectionTerm `json:"terms"`
}

// SectionTerm is one parsed row of the standard 9-column terminology table.
// It is still used for the bulk Excel export, even though the UI no longer
// shows per-row buttons.
type SectionTerm struct {
	Key             string   `json:"key"`
	StandardTerm    string   `json:"standard_term"`
	EnglishOrAbbr   string   `json:"english_or_abbr"`
	Pinyin          string   `json:"pinyin"`
	MixedScore      int      `json:"mixed_score"`
	RareScore       int      `json:"rare_score"`
	GlyphScore      int      `json:"glyph_score"`
	Level           string   `json:"level"`
	CommonMisrecs   []string `json:"common_misrecs"`
	Notes           string   `json:"notes"`
	SubsectionTitle string   `json:"subsection_title"`
	SourcePath      string   `json:"source_path"`
}
