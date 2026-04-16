package sensitive

import "errors"

var (
	ErrSensitiveDictNotFound      = errors.New("sensitive dict not found")
	ErrSensitiveEntryNotFound     = errors.New("sensitive entry not found")
	ErrSensitiveBaseDictProtected = errors.New("sensitive base dict protected")
	ErrSensitiveBaseDictConflict  = errors.New("sensitive base dict conflict")
)
