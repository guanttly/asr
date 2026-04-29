package openplatform

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
)

var (
	ErrSkillCallbackUnreachable    = errors.New("skill callback unreachable")
	ErrSkillCallbackNotWhitelisted = errors.New("skill callback not whitelisted")
)

func (s *Service) CreateSkill(ctx context.Context, app *openplatformdomain.App, req *CreateSkillRequest) (*SkillResponse, error) {
	prepared, err := s.prepareSkillPayload(app, req.Name, req.DisplayName, req.Description, req.IntentPatterns, req.ParametersSchema, req.CallbackURL, req.CallbackTimeoutMs, req.Enabled)
	if err != nil {
		return nil, err
	}
	if err := s.probeCallback(ctx, prepared.callbackURL); err != nil {
		return nil, err
	}
	skillUID, err := randomToken("skl", 12)
	if err != nil {
		return nil, err
	}
	skill := &openplatformdomain.Skill{
		SkillUID:          skillUID,
		AppID:             app.ID,
		Name:              prepared.name,
		DisplayName:       prepared.displayName,
		Description:       prepared.description,
		IntentPatterns:    prepared.intentPatterns,
		ParametersSchema:  prepared.parametersSchema,
		CallbackURL:       prepared.callbackURL,
		CallbackTimeoutMs: prepared.callbackTimeoutMs,
		Enabled:           prepared.enabled,
	}
	if err := s.skillRepo.Create(ctx, skill); err != nil {
		return nil, err
	}
	resp := toSkillResponse(skill)
	return &resp, nil
}

func (s *Service) ListSkills(ctx context.Context, app *openplatformdomain.App) ([]SkillResponse, error) {
	items, err := s.skillRepo.ListByApp(ctx, app.ID)
	if err != nil {
		return nil, err
	}
	responses := make([]SkillResponse, len(items))
	for i := range items {
		responses[i] = toSkillResponse(items[i])
	}
	return responses, nil
}

func (s *Service) GetSkill(ctx context.Context, app *openplatformdomain.App, skillID string) (*SkillResponse, error) {
	skill, err := s.skillRepo.GetByUID(ctx, strings.TrimSpace(skillID))
	if err != nil {
		return nil, err
	}
	if skill.AppID != app.ID {
		return nil, openplatformdomain.ErrSkillNotFound
	}
	resp := toSkillResponse(skill)
	return &resp, nil
}

func (s *Service) UpdateSkill(ctx context.Context, app *openplatformdomain.App, skillID string, req *UpdateSkillRequest) (*SkillResponse, error) {
	skill, err := s.skillRepo.GetByUID(ctx, strings.TrimSpace(skillID))
	if err != nil {
		return nil, err
	}
	if skill.AppID != app.ID {
		return nil, openplatformdomain.ErrSkillNotFound
	}
	prepared, err := s.prepareSkillPayload(app, req.Name, req.DisplayName, req.Description, req.IntentPatterns, req.ParametersSchema, req.CallbackURL, req.CallbackTimeoutMs, req.Enabled)
	if err != nil {
		return nil, err
	}
	if err := s.probeCallback(ctx, prepared.callbackURL); err != nil {
		return nil, err
	}
	skill.Name = prepared.name
	skill.DisplayName = prepared.displayName
	skill.Description = prepared.description
	skill.IntentPatterns = prepared.intentPatterns
	skill.ParametersSchema = prepared.parametersSchema
	skill.CallbackURL = prepared.callbackURL
	skill.CallbackTimeoutMs = prepared.callbackTimeoutMs
	skill.Enabled = prepared.enabled
	if err := s.skillRepo.Update(ctx, skill); err != nil {
		return nil, err
	}
	resp := toSkillResponse(skill)
	return &resp, nil
}

func (s *Service) DeleteSkill(ctx context.Context, app *openplatformdomain.App, skillID string) error {
	skill, err := s.skillRepo.GetByUID(ctx, strings.TrimSpace(skillID))
	if err != nil {
		return err
	}
	if skill.AppID != app.ID {
		return openplatformdomain.ErrSkillNotFound
	}
	return s.skillRepo.Delete(ctx, skill.ID)
}

func (s *Service) DryRunSkill(ctx context.Context, app *openplatformdomain.App, skillID string, utterance string) (*SkillDryRunResponse, error) {
	skill, err := s.skillRepo.GetByUID(ctx, strings.TrimSpace(skillID))
	if err != nil {
		return nil, err
	}
	if skill.AppID != app.ID {
		return nil, openplatformdomain.ErrSkillNotFound
	}
	result := &SkillDryRunResponse{Matched: false, WouldCallback: skill.CallbackURL}
	if _, matchedPattern := matchSkillUtterance([]*openplatformdomain.Skill{skill}, utterance); matchedPattern != "" {
		result.Matched = true
		result.MatchedPattern = matchedPattern
		result.ExtractedParameters = map[string]any{}
	}
	return result, nil
}

type preparedSkillPayload struct {
	name              string
	displayName       string
	description       string
	intentPatterns    []string
	parametersSchema  string
	callbackURL       string
	callbackTimeoutMs uint32
	enabled           bool
}

func (s *Service) prepareSkillPayload(app *openplatformdomain.App, name, displayName, description string, intentPatterns []string, parametersSchema, callbackURL string, callbackTimeoutMs uint32, enabled *bool) (*preparedSkillPayload, error) {
	prepared := &preparedSkillPayload{
		name:              strings.TrimSpace(name),
		displayName:       strings.TrimSpace(displayName),
		description:       strings.TrimSpace(description),
		intentPatterns:    normalizeStringSlice(intentPatterns),
		parametersSchema:  strings.TrimSpace(parametersSchema),
		callbackURL:       strings.TrimSpace(callbackURL),
		callbackTimeoutMs: callbackTimeoutMs,
		enabled:           true,
	}
	if enabled != nil {
		prepared.enabled = *enabled
	}
	if prepared.name == "" || prepared.displayName == "" {
		return nil, &ValidationError{message: "name and display_name are required"}
	}
	if len(prepared.intentPatterns) == 0 {
		return nil, &ValidationError{message: "intent_patterns is required"}
	}
	if prepared.callbackURL == "" {
		return nil, &ValidationError{message: "callback_url is required"}
	}
	if prepared.callbackTimeoutMs == 0 {
		prepared.callbackTimeoutMs = 3000
	}
	if prepared.callbackTimeoutMs > 10000 {
		return nil, &ValidationError{message: "callback_timeout_ms cannot exceed 10000"}
	}
	if !containsString(app.AllowedCaps, "skill.invoke") {
		return nil, &ValidationError{message: "app must enable skill.invoke before registering skills"}
	}
	if err := ensureCallbackWhitelisted(app.CallbackWhitelist, prepared.callbackURL); err != nil {
		return nil, err
	}
	return prepared, nil
}

func ensureCallbackWhitelisted(whitelist []string, callbackURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(callbackURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return &ValidationError{message: "callback_url is invalid"}
	}
	if len(whitelist) == 0 {
		return ErrSkillCallbackNotWhitelisted
	}
	for _, entry := range whitelist {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(callbackURL, trimmed) {
			return nil
		}
	}
	return ErrSkillCallbackNotWhitelisted
}

func (s *Service) probeCallback(ctx context.Context, callbackURL string) error {
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, callbackURL, nil)
	if err != nil {
		return ErrSkillCallbackUnreachable
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return ErrSkillCallbackUnreachable
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ErrSkillCallbackUnreachable
	}
	return nil
}

func toSkillResponse(skill *openplatformdomain.Skill) SkillResponse {
	return SkillResponse{
		SkillID:             skill.SkillUID,
		Name:                skill.Name,
		DisplayName:         skill.DisplayName,
		Description:         skill.Description,
		IntentPatterns:      append([]string(nil), skill.IntentPatterns...),
		ParametersSchema:    skill.ParametersSchema,
		CallbackURL:         skill.CallbackURL,
		CallbackTimeoutMs:   skill.CallbackTimeoutMs,
		Enabled:             skill.Enabled,
		ConsecutiveFailures: skill.ConsecutiveFailures,
		LastFailureAt:       skill.LastFailureAt,
		CreatedAt:           skill.CreatedAt,
		UpdatedAt:           skill.UpdatedAt,
	}
}
