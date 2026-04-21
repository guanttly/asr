package filler

import "time"

// Dict represents a filler-word dictionary for a specific scene.
type Dict struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	Scene       string    `json:"scene"`
	Description string    `json:"description"`
	IsBase      bool      `json:"is_base"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Entry is a single filler word under a dictionary.
type Entry struct {
	ID        uint64    `json:"id"`
	DictID    uint64    `json:"dict_id"`
	Word      string    `json:"word"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
