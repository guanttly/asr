package voicecommand

import "time"

// Dict defines a group of voice control commands.
type Dict struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	GroupKey    string    `json:"group_key"`
	Description string    `json:"description"`
	IsBase      bool      `json:"is_base"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Entry defines one normalized command intent with multiple utterance examples.
type Entry struct {
	ID         uint64    `json:"id"`
	DictID     uint64    `json:"dict_id"`
	Intent     string    `json:"intent"`
	Label      string    `json:"label"`
	Utterances []string  `json:"utterances"`
	Enabled    bool      `json:"enabled"`
	SortOrder  int       `json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
