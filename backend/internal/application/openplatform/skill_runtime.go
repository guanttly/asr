package openplatform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
)

func (s *Service) MatchAndInvokeSkill(ctx context.Context, ownerUserID uint64, utterance string, taskID, meetingID uint64) (*openplatformdomain.SkillInvokeResult, error) {
	appID, ok := openplatformdomain.AppIDFromOwnerUserID(ownerUserID)
	if !ok {
		return nil, nil
	}
	app, err := s.appRepo.GetByID(ctx, appID)
	if err != nil {
		if errors.Is(err, openplatformdomain.ErrAppNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if app.Status != openplatformdomain.AppStatusActive || !containsString(app.AllowedCaps, "skill.invoke") {
		return nil, nil
	}
	skills, err := s.skillRepo.ListByApp(ctx, app.ID)
	if err != nil {
		return nil, err
	}
	skill, matchedPattern := matchSkillUtterance(skills, utterance)
	if skill == nil {
		return nil, nil
	}
	requestID, err := randomToken("req", 12)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"request_id":      requestID,
		"skill_id":        skill.SkillUID,
		"skill_name":      skill.Name,
		"display_name":    skill.DisplayName,
		"matched_pattern": matchedPattern,
		"utterance":       strings.TrimSpace(utterance),
		"parameters":      map[string]any{},
		"context":         buildSkillInvokeContext(taskID, meetingID),
		"ts":              time.Now().UTC().Format(time.RFC3339),
	}

	callbackCtx := ctx
	var cancel context.CancelFunc
	if skill.CallbackTimeoutMs > 0 {
		callbackCtx, cancel = context.WithTimeout(ctx, time.Duration(skill.CallbackTimeoutMs)*time.Millisecond)
		defer cancel()
	}

	startedAt := time.Now()
	responseBody, httpStatus, dispatchErr := s.dispatchOpenCallbackWithBackoffs(callbackCtx, app, skill.CallbackURL, payload, map[string]string{
		"X-OpenAPI-Request-Id": requestID,
		"X-OpenAPI-Skill-Id":   skill.SkillUID,
	}, []time.Duration{0})

	status := invocationStatusFromCallback(dispatchErr, httpStatus)
	result := &openplatformdomain.SkillInvokeResult{
		SkillID:        skill.SkillUID,
		SkillName:      skill.Name,
		MatchedPattern: matchedPattern,
		Status:         status,
		ResponseJSON:   strings.TrimSpace(string(responseBody)),
	}
	if httpStatus > 0 {
		statusCopy := httpStatus
		result.HTTPStatus = &statusCopy
	}
	if dispatchErr != nil {
		result.ErrorMessage = dispatchErr.Error()
	}

	if err := s.recordSkillInvocation(ctx, skill, requestID, matchedPattern, utterance, status, httpStatus, uint32(time.Since(startedAt).Milliseconds()), result.ErrorMessage); err != nil {
		return nil, err
	}
	if err := s.applySkillInvocationOutcome(ctx, skill, status); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) recordSkillInvocation(ctx context.Context, skill *openplatformdomain.Skill, requestID, matchedPattern, utterance string, status openplatformdomain.InvocationStatus, httpStatus uint16, latencyMs uint32, errorMessage string) error {
	if s.invocationRepo == nil || skill == nil {
		return nil
	}
	invocation := &openplatformdomain.SkillInvocation{
		SkillID:        skill.ID,
		AppID:          skill.AppID,
		RequestID:      requestID,
		MatchedPattern: matchedPattern,
		Utterance:      strings.TrimSpace(utterance),
		ParametersJSON: "{}",
		Status:         status,
		ErrorMessage:   strings.TrimSpace(errorMessage),
	}
	if httpStatus > 0 {
		statusCopy := httpStatus
		invocation.HTTPStatus = &statusCopy
	}
	latencyCopy := latencyMs
	invocation.LatencyMs = &latencyCopy
	return s.invocationRepo.Create(ctx, invocation)
}

func (s *Service) applySkillInvocationOutcome(ctx context.Context, skill *openplatformdomain.Skill, status openplatformdomain.InvocationStatus) error {
	if skill == nil {
		return nil
	}
	updated := false
	if status == openplatformdomain.InvocationStatusSuccess {
		if skill.ConsecutiveFailures != 0 || skill.LastFailureAt != nil {
			skill.ConsecutiveFailures = 0
			skill.LastFailureAt = nil
			updated = true
		}
	} else {
		now := time.Now()
		skill.ConsecutiveFailures++
		skill.LastFailureAt = &now
		if s.skillFailureLimit > 0 && skill.ConsecutiveFailures >= s.skillFailureLimit {
			skill.Enabled = false
		}
		updated = true
	}
	if !updated {
		return nil
	}
	return s.skillRepo.Update(ctx, skill)
}

func matchSkillUtterance(skills []*openplatformdomain.Skill, utterance string) (*openplatformdomain.Skill, string) {
	trimmedUtterance := strings.TrimSpace(utterance)
	if trimmedUtterance == "" {
		return nil, ""
	}
	normalizedUtterance := normalizeSkillText(trimmedUtterance)
	var bestSkill *openplatformdomain.Skill
	bestPattern := ""
	bestScore := 0
	for _, skill := range skills {
		if skill == nil || !skill.Enabled {
			continue
		}
		for _, pattern := range skill.IntentPatterns {
			score := scoreSkillPatternMatch(trimmedUtterance, normalizedUtterance, pattern)
			if score <= 0 {
				continue
			}
			if score > bestScore || (score == bestScore && len(pattern) > len(bestPattern)) {
				bestSkill = skill
				bestPattern = pattern
				bestScore = score
			}
		}
	}
	return bestSkill, bestPattern
}

func scoreSkillPatternMatch(utterance string, normalizedUtterance string, pattern string) int {
	trimmedPattern := strings.TrimSpace(pattern)
	if trimmedPattern == "" {
		return 0
	}
	if regexBody, ok := skillRegexPattern(trimmedPattern); ok {
		re, err := regexp.Compile(regexBody)
		if err != nil {
			return 0
		}
		if re.MatchString(utterance) {
			return 3000 + len(regexBody)
		}
		return 0
	}
	normalizedPattern := normalizeSkillText(trimmedPattern)
	if normalizedPattern == "" {
		return 0
	}
	if normalizedUtterance == normalizedPattern {
		return 2000 + len(normalizedPattern)
	}
	if strings.Contains(normalizedUtterance, normalizedPattern) {
		return 1000 + len(normalizedPattern)
	}
	if len([]rune(normalizedUtterance)) >= 2 && strings.Contains(normalizedPattern, normalizedUtterance) {
		return 500 + len(normalizedUtterance)
	}
	return 0
}

func skillRegexPattern(pattern string) (string, bool) {
	trimmed := strings.TrimSpace(pattern)
	for _, prefix := range []string{"re:", "regexp:"} {
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)), true
		}
	}
	return "", false
}

func normalizeSkillText(value string) string {
	replacer := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\n", "",
		"，", "",
		",", "",
		"。", "",
		".", "",
		"！", "",
		"!", "",
		"？", "",
		"?", "",
		"、", "",
		"-", "",
		"_", "",
		"(", "",
		")", "",
		"（", "",
		"）", "",
		"：", "",
		":", "",
		"；", "",
		";", "",
		"【", "",
		"】", "",
		"[", "",
		"]", "",
		"\"", "",
		"'", "",
		"“", "",
		"”", "",
		"‘", "",
		"’", "",
	)
	return replacer.Replace(strings.ToLower(strings.TrimSpace(value)))
}

func buildSkillInvokeContext(taskID, meetingID uint64) map[string]any {
	ctx := map[string]any{}
	if taskID > 0 {
		ctx["task_id"] = taskID
	}
	if meetingID > 0 {
		ctx["meeting_id"] = meetingID
	}
	return ctx
}

func invocationStatusFromCallback(err error, httpStatus uint16) openplatformdomain.InvocationStatus {
	if err == nil {
		return openplatformdomain.InvocationStatusSuccess
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return openplatformdomain.InvocationStatusTimeout
	}
	if httpStatus == http.StatusUnauthorized || httpStatus == http.StatusForbidden {
		return openplatformdomain.InvocationStatusSignedRejected
	}
	return openplatformdomain.InvocationStatusFailed
}

func marshalSkillInvokeDetail(result *openplatformdomain.SkillInvokeResult) json.RawMessage {
	if result == nil {
		return nil
	}
	payload := map[string]any{
		"match_mode":        "open_skill",
		"matched":           true,
		"group_key":         "open_skill",
		"intent":            result.SkillName,
		"skill_id":          result.SkillID,
		"skill_name":        result.SkillName,
		"matched_pattern":   result.MatchedPattern,
		"invocation_status": result.Status,
		"error_message":     result.ErrorMessage,
	}
	if result.HTTPStatus != nil {
		payload["http_status"] = *result.HTTPStatus
	}
	if strings.TrimSpace(result.ResponseJSON) != "" {
		var response any
		if err := json.Unmarshal([]byte(result.ResponseJSON), &response); err == nil {
			payload["callback_response"] = response
		} else {
			payload["callback_response_raw"] = result.ResponseJSON
		}
	}
	detail, _ := json.Marshal(payload)
	return detail
}

func skillInvokeReason(result *openplatformdomain.SkillInvokeResult) string {
	if result == nil {
		return "未命中开放技能"
	}
	name := strings.TrimSpace(result.SkillName)
	if name == "" {
		name = strings.TrimSpace(result.SkillID)
	}
	switch result.Status {
	case openplatformdomain.InvocationStatusSuccess:
		return fmt.Sprintf("已触发开放技能：%s", name)
	case openplatformdomain.InvocationStatusTimeout:
		return fmt.Sprintf("已命中开放技能但回调超时：%s", name)
	case openplatformdomain.InvocationStatusSignedRejected:
		return fmt.Sprintf("已命中开放技能但签名被拒绝：%s", name)
	default:
		return fmt.Sprintf("已命中开放技能但回调失败：%s", name)
	}
}
