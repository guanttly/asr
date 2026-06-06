package terminology

import "time"

// TermDict represents a terminology dictionary scoped to a domain (e.g. medical, legal).
type TermDict struct {
	ID                     uint64      `json:"id"`
	Name                   string      `json:"name"`
	Domain                 string      `json:"domain"` // e.g. "医疗", "法律"
	RuleProcessingEnabled  bool        `json:"rule_processing_enabled"`
	TextReplacementEnabled bool        `json:"text_replacement_enabled"`
	Entries                []TermEntry `json:"entries,omitempty"`
	CreatedAt              time.Time   `json:"created_at"`
	UpdatedAt              time.Time   `json:"updated_at"`
}

// TermEntry is a single correct term with its known wrong variants.
type TermEntry struct {
	ID            uint64   `json:"id"`
	DictID        uint64   `json:"dict_id"`
	CorrectTerm   string   `json:"correct_term"`
	WrongVariants []string `json:"wrong_variants"` // stored as JSON array
}

// RuleMatchType identifies how a correction rule should be applied.
type RuleMatchType string

const (
	RuleMatchLiteral           RuleMatchType = "literal"
	RuleMatchRegex             RuleMatchType = "regex"
	RuleMatchNumberNormalize   RuleMatchType = "number_normalize"
	RuleMatchHallucinationTrim RuleMatchType = "hallucination_trim"
)

// CorrectionRule defines a specific correction rule.
type CorrectionRule struct {
	ID            uint64        `json:"id"`
	DictID        uint64        `json:"dict_id"`
	MatchType     RuleMatchType `json:"match_type"`
	Pattern       string        `json:"pattern"`
	Replacement   string        `json:"replacement"`
	Enabled       bool          `json:"enabled"`
	SortOrder     int           `json:"sort_order"`
	Priority      int           `json:"priority"`
	ConflictGroup string        `json:"conflict_group"`
	CreatedAt     time.Time     `json:"created_at"`
}
