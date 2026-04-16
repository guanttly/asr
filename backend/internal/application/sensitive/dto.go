package sensitive

// CreateDictRequest creates a sensitive dictionary.
type CreateDictRequest struct {
	Name        string `json:"name" binding:"required"`
	Scene       string `json:"scene" binding:"required"`
	Description string `json:"description"`
	IsBase      bool   `json:"is_base"`
}

// UpdateDictRequest updates a sensitive dictionary.
type UpdateDictRequest struct {
	Name        string `json:"name" binding:"required"`
	Scene       string `json:"scene" binding:"required"`
	Description string `json:"description"`
	IsBase      bool   `json:"is_base"`
}

// DictResponse is the DTO for a sensitive dictionary.
type DictResponse struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Scene       string `json:"scene"`
	Description string `json:"description"`
	IsBase      bool   `json:"is_base"`
}

// CreateEntryRequest creates a sensitive word entry.
type CreateEntryRequest struct {
	DictID  uint64 `json:"dict_id"`
	Word    string `json:"word" binding:"required"`
	Enabled bool   `json:"enabled"`
}

// UpdateEntryRequest updates a sensitive word entry.
type UpdateEntryRequest struct {
	ID      uint64 `json:"id"`
	DictID  uint64 `json:"dict_id"`
	Word    string `json:"word" binding:"required"`
	Enabled bool   `json:"enabled"`
}

// EntryResponse is the DTO for a sensitive word entry.
type EntryResponse struct {
	ID      uint64 `json:"id"`
	Word    string `json:"word"`
	Enabled bool   `json:"enabled"`
}
