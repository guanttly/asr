package rulescatalog

// TreeNode is one entry in the rules catalog directory tree.
type TreeNode struct {
	Name       string     `json:"name"`
	Path       string     `json:"path"`
	IsDir      bool       `json:"is_dir"`
	Title      string     `json:"title,omitempty"`
	ExcelPath  string     `json:"excel_path,omitempty"`
	TotalRules int        `json:"total_rules,omitempty"`
	EnabledCnt int        `json:"enabled_count,omitempty"`
	RegexCnt   int        `json:"regex_count,omitempty"`
	LiteralCnt int        `json:"literal_count,omitempty"`
	Children   []TreeNode `json:"children,omitempty"`
}

// FileDetail bundles the markdown body with parsed rules for a single file.
type FileDetail struct {
	Path         string        `json:"path"`
	Name         string        `json:"name"`
	Title        string        `json:"title"`
	MarkdownBody string        `json:"markdown_body"`
	Rules        []SectionRule `json:"rules"`
}

// SectionRule is one parsed row of the 9-column rules table.
type SectionRule struct {
	Key             string `json:"key"`
	Category        string `json:"category"`
	Pattern         string `json:"pattern"`
	Replacement     string `json:"replacement"`
	MatchType       string `json:"match_type"`
	Priority        int    `json:"priority"`
	ConflictGroup   string `json:"conflict_group"`
	Enabled         bool   `json:"enabled"`
	Example         string `json:"example"`
	Notes           string `json:"notes"`
	SubsectionTitle string `json:"subsection_title"`
	SourcePath      string `json:"source_path"`
}
