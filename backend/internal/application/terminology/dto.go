package terminology

// CreateDictRequest is the DTO for creating a terminology dictionary.
type CreateDictRequest struct {
	Name   string `json:"name" binding:"required"`
	Domain string `json:"domain" binding:"required"`
}

// UpdateDictRequest is the DTO for updating a dictionary.
type UpdateDictRequest struct {
	Name   string `json:"name" binding:"required"`
	Domain string `json:"domain" binding:"required"`
}

// CreateEntryRequest is the DTO for adding a term entry.
type CreateEntryRequest struct {
	DictID        uint64   `json:"dict_id"`
	CorrectTerm   string   `json:"correct_term" binding:"required"`
	WrongVariants []string `json:"wrong_variants"`
}

// UpdateEntryRequest is the DTO for updating a term entry.
type UpdateEntryRequest struct {
	ID            uint64   `json:"id"`
	DictID        uint64   `json:"dict_id"`
	CorrectTerm   string   `json:"correct_term" binding:"required"`
	WrongVariants []string `json:"wrong_variants"`
}

// BatchImportRequest supports importing multiple entries at once.
type BatchImportRequest struct {
	DictID  uint64               `json:"dict_id"`
	Entries []CreateEntryRequest `json:"entries" binding:"required,dive"`
}

type BatchImportResult struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

// DictResponse is the DTO for a dictionary.
type DictResponse struct {
	ID     uint64 `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// EntryResponse is the DTO for a term entry.
type EntryResponse struct {
	ID            uint64   `json:"id"`
	CorrectTerm   string   `json:"correct_term"`
	WrongVariants []string `json:"wrong_variants"`
}

// RuleResponse is the DTO for a correction rule.
type RuleResponse struct {
	ID            uint64 `json:"id"`
	MatchType     string `json:"match_type"`
	Pattern       string `json:"pattern"`
	Replacement   string `json:"replacement"`
	Enabled       bool   `json:"enabled"`
	SortOrder     int    `json:"sort_order"`
	Priority      int    `json:"priority"`
	ConflictGroup string `json:"conflict_group"`
}

// CreateRuleRequest is the DTO for adding a correction rule.
type CreateRuleRequest struct {
	DictID        uint64 `json:"dict_id"`
	MatchType     string `json:"match_type"`
	Pattern       string `json:"pattern"`
	Replacement   string `json:"replacement"`
	Enabled       bool   `json:"enabled"`
	SortOrder     int    `json:"sort_order"`
	Priority      int    `json:"priority"`
	ConflictGroup string `json:"conflict_group"`
}

// UpdateRuleRequest is the DTO for updating a correction rule.
type UpdateRuleRequest struct {
	ID            uint64 `json:"id"`
	DictID        uint64 `json:"dict_id"`
	MatchType     string `json:"match_type"`
	Pattern       string `json:"pattern"`
	Replacement   string `json:"replacement"`
	Enabled       bool   `json:"enabled"`
	SortOrder     int    `json:"sort_order"`
	Priority      int    `json:"priority"`
	ConflictGroup string `json:"conflict_group"`
}
