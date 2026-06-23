package sensitive

import "errors"

var (
	ErrSensitiveDictNotFound      = errors.New("sensitive dict not found")
	ErrSensitiveEntryNotFound     = errors.New("sensitive entry not found")
	ErrSensitiveEntryDuplicate    = errors.New("sensitive entry duplicate")
	ErrSensitiveBaseDictProtected = errors.New("sensitive base dict protected")
	ErrSensitiveBaseDictConflict  = errors.New("sensitive base dict conflict")
	ErrSensitiveDictInUse         = errors.New("sensitive dict in use")
)
