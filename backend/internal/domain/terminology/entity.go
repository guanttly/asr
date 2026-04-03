package terminology

import "time"

// TermDict represents a terminology dictionary scoped to a domain (e.g. medical, legal).
type TermDict struct {
	ID        uint64      `json:"id"`
	Name      string      `json:"name"`
	Domain    string      `json:"domain"` // e.g. "医疗", "法律"
	Entries   []TermEntry `json:"entries,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// TermEntry is a single correct term with its known wrong variants.
type TermEntry struct {
	ID            uint64   `json:"id"`
	DictID        uint64   `json:"dict_id"`
	CorrectTerm   string   `json:"correct_term"`
	WrongVariants []string `json:"wrong_variants"` // stored as JSON array
	Pinyin        string   `json:"pinyin"`
}

// CorrectionLayer identifies which correction pipeline layer a rule belongs to.
type CorrectionLayer int

const (
	LayerExactMatch    CorrectionLayer = 1
	LayerEditDistance  CorrectionLayer = 2
	LayerPinyinSimilar CorrectionLayer = 3
)

// CorrectionRule defines a specific correction rule.
type CorrectionRule struct {
	ID          uint64          `json:"id"`
	DictID      uint64          `json:"dict_id"`
	Layer       CorrectionLayer `json:"layer"`
	Pattern     string          `json:"pattern"`
	Replacement string          `json:"replacement"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
}
