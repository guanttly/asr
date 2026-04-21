package filler

import "errors"

var (
	ErrFillerDictNotFound      = errors.New("filler dict not found")
	ErrFillerEntryNotFound     = errors.New("filler entry not found")
	ErrFillerBaseDictProtected = errors.New("filler base dict protected")
	ErrFillerBaseDictConflict  = errors.New("filler base dict conflict")
)
