package errcode

const (
	CodeBadRequest   = 40000
	CodeUnauthorized = 40100
	CodeForbidden    = 40300
	CodeNotFound     = 40400
	CodeInternal     = 50000
)

const (
	OpenValidation                  = "ERR_VALIDATION"
	OpenInternal                    = "ERR_OPEN_INTERNAL"
	OpenAuthMissing                 = "ERR_OPEN_AUTH_MISSING"
	OpenAuthInvalid                 = "ERR_OPEN_AUTH_INVALID"
	OpenAuthExpired                 = "ERR_OPEN_AUTH_EXPIRED"
	OpenAuthRevoked                 = "ERR_OPEN_AUTH_REVOKED"
	OpenAppDisabled                 = "ERR_OPEN_APP_DISABLED"
	OpenCapDenied                   = "ERR_OPEN_CAP_DENIED"
	OpenRateLimited                 = "ERR_OPEN_RATE_LIMITED"
	OpenWorkflowNotFound            = "ERR_WORKFLOW_NOT_FOUND"
	OpenWorkflowInvalid             = "ERR_WORKFLOW_INVALID"
	OpenEditionLimited              = "ERR_EDITION_LIMITED"
	OpenASRPartial                  = "ERR_ASR_PARTIAL"
	OpenAudioTooLarge               = "ERR_AUDIO_TOO_LARGE"
	OpenAudioTooLong                = "ERR_AUDIO_TOO_LONG"
	OpenUnsupportedFormat           = "ERR_UNSUPPORTED_FORMAT"
	OpenSessionExpired              = "ERR_SESSION_EXPIRED"
	OpenSkillNameDuplicated         = "ERR_SKILL_NAME_DUPLICATED"
	OpenSkillNotFound               = "ERR_SKILL_NOT_FOUND"
	OpenSkillCallbackUnreachable    = "ERR_SKILL_CALLBACK_UNREACHABLE"
	OpenSkillCallbackNotWhitelisted = "ERR_SKILL_CALLBACK_NOT_WHITELISTED"
	OpenSkillDisabledByFailure      = "ERR_SKILL_DISABLED_BY_FAILURE"
	OpenTemplateNotFound            = "ERR_TEMPLATE_NOT_FOUND"
)
