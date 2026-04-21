package voicecommand

import "errors"

var (
	ErrVoiceCommandDictNotFound      = errors.New("voice command dict not found")
	ErrVoiceCommandEntryNotFound     = errors.New("voice command entry not found")
	ErrVoiceCommandBaseDictProtected = errors.New("voice command base dict protected")
	ErrVoiceCommandBaseDictConflict  = errors.New("voice command base dict conflict")
)
