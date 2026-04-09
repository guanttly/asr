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
	Pinyin        string   `json:"pinyin"`
}

// UpdateEntryRequest is the DTO for updating a term entry.
type UpdateEntryRequest struct {
	ID            uint64   `json:"id"`
	DictID        uint64   `json:"dict_id"`
	CorrectTerm   string   `json:"correct_term" binding:"required"`
	WrongVariants []string `json:"wrong_variants"`
	Pinyin        string   `json:"pinyin"`
}

// BatchImportRequest supports importing multiple entries at once.
type BatchImportRequest struct {
	DictID  uint64               `json:"dict_id"`
	Entries []CreateEntryRequest `json:"entries" binding:"required,dive"`
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
	Pinyin        string   `json:"pinyin"`
}

// RuleResponse is the DTO for a correction rule.
type RuleResponse struct {
	ID          uint64 `json:"id"`
	Layer       int    `json:"layer"`
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
	Enabled     bool   `json:"enabled"`
}

// CreateRuleRequest is the DTO for adding a correction rule.
type CreateRuleRequest struct {
	DictID      uint64 `json:"dict_id"`
	Layer       int    `json:"layer" binding:"required,oneof=1 2 3"`
	Pattern     string `json:"pattern" binding:"required"`
	Replacement string `json:"replacement" binding:"required"`
	Enabled     bool   `json:"enabled"`
}

// UpdateRuleRequest is the DTO for updating a correction rule.
type UpdateRuleRequest struct {
	ID          uint64 `json:"id"`
	DictID      uint64 `json:"dict_id"`
	Layer       int    `json:"layer" binding:"required,oneof=1 2 3"`
	Pattern     string `json:"pattern" binding:"required"`
	Replacement string `json:"replacement" binding:"required"`
	Enabled     bool   `json:"enabled"`
}
