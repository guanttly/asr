package voicecommand

type CreateDictRequest struct {
	Name        string `json:"name" binding:"required"`
	GroupKey    string `json:"group_key" binding:"required"`
	Description string `json:"description"`
	IsBase      bool   `json:"is_base"`
}

type UpdateDictRequest struct {
	Name        string `json:"name" binding:"required"`
	GroupKey    string `json:"group_key" binding:"required"`
	Description string `json:"description"`
	IsBase      bool   `json:"is_base"`
}

type DictResponse struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	GroupKey    string `json:"group_key"`
	Description string `json:"description"`
	IsBase      bool   `json:"is_base"`
}

type CreateEntryRequest struct {
	DictID     uint64   `json:"dict_id"`
	Intent     string   `json:"intent" binding:"required"`
	Label      string   `json:"label" binding:"required"`
	Utterances []string `json:"utterances" binding:"required,min=1"`
	Enabled    bool     `json:"enabled"`
	SortOrder  int      `json:"sort_order"`
}

type UpdateEntryRequest struct {
	ID         uint64   `json:"id"`
	DictID     uint64   `json:"dict_id"`
	Intent     string   `json:"intent" binding:"required"`
	Label      string   `json:"label" binding:"required"`
	Utterances []string `json:"utterances" binding:"required,min=1"`
	Enabled    bool     `json:"enabled"`
	SortOrder  int      `json:"sort_order"`
}

type EntryResponse struct {
	ID         uint64   `json:"id"`
	Intent     string   `json:"intent"`
	Label      string   `json:"label"`
	Utterances []string `json:"utterances"`
	Enabled    bool     `json:"enabled"`
	SortOrder  int      `json:"sort_order"`
}
