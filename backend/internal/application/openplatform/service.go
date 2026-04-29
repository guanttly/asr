package openplatform

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
	"golang.org/x/crypto/argon2"
)

const (
	defaultTokenExpiresInSec = int64(7200)
	defaultRateLimitPerSec   = uint32(30)
	argonTime                = uint32(3)
	argonMemory              = uint32(64 * 1024)
	argonThreads             = uint8(2)
	argonKeyLen              = uint32(32)
	argonSaltLen             = 16
)

var (
	ErrOpenAuthMissing      = errors.New("missing open auth token")
	ErrOpenAuthInvalid      = errors.New("invalid open auth token")
	ErrOpenAuthExpired      = errors.New("expired open auth token")
	ErrOpenAuthRevoked      = errors.New("revoked open auth token")
	ErrOpenAppDisabled      = errors.New("open app disabled")
	ErrOpenCapDenied        = errors.New("open capability denied")
	ErrCallbackWhitelistReq = errors.New("callback whitelist required when skill.invoke is enabled")
)

type ValidationError struct {
	message string
}

func (e *ValidationError) Error() string {
	return e.message
}

type RateLimitError struct {
	RetryAfterSec int
}

func (e *RateLimitError) Error() string {
	return "open api rate limited"
}

type workflowValidator interface {
	ValidateWorkflowBinding(ctx context.Context, workflowID uint64, expectedType wfdomain.WorkflowType) (*wfdomain.Workflow, error)
}

type Service struct {
	appRepo               openplatformdomain.AppRepository
	skillRepo             openplatformdomain.SkillRepository
	callLogRepo           openplatformdomain.CallLogRepository
	invocationRepo        openplatformdomain.SkillInvocationRepository
	workflowValidator     workflowValidator
	platformSecret        string
	tokenExpiresIn        int64
	rateLimiter           *appRateLimiter
	callbackHTTPClient    *http.Client
	callbackRetryBackoffs []time.Duration
	skillFailureLimit     uint32
}

func NewService(appRepo openplatformdomain.AppRepository, skillRepo openplatformdomain.SkillRepository, callLogRepo openplatformdomain.CallLogRepository, workflowValidator workflowValidator, platformSecret string, tokenExpiresIn int64) *Service {
	if strings.TrimSpace(platformSecret) == "" {
		platformSecret = "open-platform-dev-secret"
	}
	if tokenExpiresIn <= 0 {
		tokenExpiresIn = defaultTokenExpiresInSec
	}
	return &Service{
		appRepo:               appRepo,
		skillRepo:             skillRepo,
		callLogRepo:           callLogRepo,
		workflowValidator:     workflowValidator,
		platformSecret:        platformSecret,
		tokenExpiresIn:        tokenExpiresIn,
		rateLimiter:           newAppRateLimiter(),
		callbackHTTPClient:    &http.Client{Timeout: 10 * time.Second},
		callbackRetryBackoffs: []time.Duration{0, time.Minute, 5 * time.Minute, 30 * time.Minute},
		skillFailureLimit:     5,
	}
}

func (s *Service) SetSkillInvocationRepo(repo openplatformdomain.SkillInvocationRepository) {
	s.invocationRepo = repo
}

var capabilityCatalog = []CapabilityDescriptor{
	{ID: "asr.recognize", DisplayName: "语音转文字", Description: "同步、VAD 与异步语音转文字能力"},
	{ID: "asr.stream", DisplayName: "实时流识别", Description: "实时音频流识别能力"},
	{ID: "meeting.summary", DisplayName: "会议纪要", Description: "音频或文本生成会议纪要"},
	{ID: "nlp.correct", DisplayName: "文本纠错", Description: "独立文本纠错能力"},
	{ID: "skill.register", DisplayName: "Skill 管理", Description: "注册、修改和删除 Skill"},
	{ID: "skill.invoke", DisplayName: "Skill 回调", Description: "接收语音指令回调"},
}

var capabilityIndex = func() map[string]CapabilityDescriptor {
	items := make(map[string]CapabilityDescriptor, len(capabilityCatalog))
	for _, item := range capabilityCatalog {
		items[item.ID] = item
	}
	return items
}()

func (s *Service) ListCapabilities() []CapabilityDescriptor {
	items := make([]CapabilityDescriptor, len(capabilityCatalog))
	copy(items, capabilityCatalog)
	return items
}

func (s *Service) ListApps(ctx context.Context, offset, limit int) ([]AppResponse, int64, error) {
	items, total, err := s.appRepo.List(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	responses := make([]AppResponse, len(items))
	for i := range items {
		responses[i] = toAppResponse(items[i])
	}
	return responses, total, nil
}

func (s *Service) GetApp(ctx context.Context, id uint64) (*AppResponse, error) {
	app, err := s.appRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	response := toAppResponse(app)
	return &response, nil
}

func (s *Service) CreateApp(ctx context.Context, ownerUserID uint64, req *CreateAppRequest) (*CreateAppResponse, error) {
	prepared, err := s.prepareAppPayload(ctx, req.Name, req.Description, req.AllowedCaps, req.DefaultWorkflows, req.CallbackWhitelist, req.RateLimitPerSec, req.MetaJSON)
	if err != nil {
		return nil, err
	}

	appID, err := randomToken("app", 12)
	if err != nil {
		return nil, err
	}
	appSecret, err := randomToken("sk", 24)
	if err != nil {
		return nil, err
	}
	hash, err := hashSecret(appSecret)
	if err != nil {
		return nil, err
	}
	ciphertext, err := s.encryptSecret(appSecret)
	if err != nil {
		return nil, err
	}

	app := &openplatformdomain.App{
		AppID:               appID,
		Name:                prepared.name,
		Description:         prepared.description,
		SecretHint:          maskSecret(appSecret),
		AppSecretHash:       hash,
		AppSecretCiphertext: ciphertext,
		SecretVersion:       1,
		Status:              openplatformdomain.AppStatusActive,
		OwnerUserID:         ownerUserID,
		RateLimitPerSec:     prepared.rateLimit,
		AllowedCaps:         prepared.allowedCaps,
		DefaultWorkflows:    prepared.defaultWorkflows,
		CallbackWhitelist:   prepared.callbackWhitelist,
		MetaJSON:            prepared.metaJSON,
	}
	if err := s.appRepo.Create(ctx, app); err != nil {
		return nil, err
	}
	result := &CreateAppResponse{AppResponse: toAppResponse(app), AppSecret: appSecret}
	return result, nil
}

func (s *Service) UpdateApp(ctx context.Context, id uint64, req *UpdateAppRequest) (*AppResponse, error) {
	app, err := s.appRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	prepared, err := s.prepareAppPayload(ctx, req.Name, req.Description, req.AllowedCaps, req.DefaultWorkflows, req.CallbackWhitelist, req.RateLimitPerSec, req.MetaJSON)
	if err != nil {
		return nil, err
	}
	app.Name = prepared.name
	app.Description = prepared.description
	app.AllowedCaps = prepared.allowedCaps
	app.DefaultWorkflows = prepared.defaultWorkflows
	app.CallbackWhitelist = prepared.callbackWhitelist
	app.RateLimitPerSec = prepared.rateLimit
	app.MetaJSON = prepared.metaJSON
	if err := s.appRepo.Update(ctx, app); err != nil {
		return nil, err
	}
	response := toAppResponse(app)
	return &response, nil
}

func (s *Service) RotateSecret(ctx context.Context, id uint64) (*CreateAppResponse, error) {
	app, err := s.appRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	appSecret, err := randomToken("sk", 24)
	if err != nil {
		return nil, err
	}
	hash, err := hashSecret(appSecret)
	if err != nil {
		return nil, err
	}
	ciphertext, err := s.encryptSecret(appSecret)
	if err != nil {
		return nil, err
	}
	app.AppSecretHash = hash
	app.AppSecretCiphertext = ciphertext
	app.SecretHint = maskSecret(appSecret)
	app.SecretVersion++
	if err := s.appRepo.Update(ctx, app); err != nil {
		return nil, err
	}
	return &CreateAppResponse{AppResponse: toAppResponse(app), AppSecret: appSecret}, nil
}

func (s *Service) DisableApp(ctx context.Context, id uint64) error {
	return s.appRepo.UpdateStatus(ctx, id, openplatformdomain.AppStatusDisabled)
}

func (s *Service) EnableApp(ctx context.Context, id uint64) error {
	return s.appRepo.UpdateStatus(ctx, id, openplatformdomain.AppStatusActive)
}

func (s *Service) RevokeApp(ctx context.Context, id uint64) error {
	return s.appRepo.UpdateStatus(ctx, id, openplatformdomain.AppStatusRevoked)
}

func (s *Service) ListAppCalls(ctx context.Context, id uint64, limit int) ([]*openplatformdomain.CallLog, error) {
	if _, err := s.appRepo.GetByID(ctx, id); err != nil {
		return nil, err
	}
	return s.callLogRepo.ListByApp(ctx, id, limit)
}

func (s *Service) IssueToken(ctx context.Context, req *IssueTokenRequest) (*IssueTokenResponse, error) {
	appID := strings.TrimSpace(req.AppID)
	appSecret := strings.TrimSpace(req.AppSecret)
	if appID == "" || appSecret == "" {
		return nil, ErrOpenAuthInvalid
	}
	app, err := s.appRepo.GetByAppID(ctx, appID)
	if err != nil {
		if errors.Is(err, openplatformdomain.ErrAppNotFound) {
			return nil, ErrOpenAuthInvalid
		}
		return nil, err
	}
	if !verifySecret(appSecret, app.AppSecretHash) {
		return nil, ErrOpenAuthInvalid
	}
	if app.Status != openplatformdomain.AppStatusActive {
		return nil, ErrOpenAppDisabled
	}
	now := time.Now()
	claims := &AccessTokenClaims{
		AppID:         app.AppID,
		AllowedCaps:   append([]string(nil), app.AllowedCaps...),
		SecretVersion: app.SecretVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(s.tokenExpiresIn) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	sort.Strings(claims.AllowedCaps)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.signingKey(app.AppID))
	if err != nil {
		return nil, err
	}
	return &IssueTokenResponse{
		AccessToken: signed,
		ExpiresIn:   s.tokenExpiresIn,
		TokenType:   "Bearer",
		AllowedCaps: claims.AllowedCaps,
	}, nil
}

func (s *Service) AuthenticateAccessToken(ctx context.Context, tokenString string, requiredCapability string) (*AccessTokenClaims, *openplatformdomain.App, error) {
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return nil, nil, ErrOpenAuthMissing
	}
	previewClaims := &AccessTokenClaims{}
	parser := jwt.NewParser()
	if _, _, err := parser.ParseUnverified(tokenString, previewClaims); err != nil {
		return nil, nil, ErrOpenAuthInvalid
	}
	if strings.TrimSpace(previewClaims.AppID) == "" {
		return nil, nil, ErrOpenAuthInvalid
	}
	app, err := s.appRepo.GetByAppID(ctx, previewClaims.AppID)
	if err != nil {
		if errors.Is(err, openplatformdomain.ErrAppNotFound) {
			return nil, nil, ErrOpenAuthInvalid
		}
		return nil, nil, err
	}
	claims := &AccessTokenClaims{}
	_, err = jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		return s.signingKey(app.AppID), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, nil, ErrOpenAuthExpired
		}
		return nil, nil, ErrOpenAuthInvalid
	}
	if app.Status != openplatformdomain.AppStatusActive {
		return nil, nil, ErrOpenAppDisabled
	}
	if claims.SecretVersion != app.SecretVersion {
		return nil, nil, ErrOpenAuthRevoked
	}
	if requiredCapability != "" {
		if !containsString(app.AllowedCaps, requiredCapability) || !containsString(claims.AllowedCaps, requiredCapability) {
			return nil, nil, ErrOpenCapDenied
		}
	}
	if !s.rateLimiter.Allow(app.AppID, app.RateLimitPerSec) {
		return nil, nil, &RateLimitError{RetryAfterSec: 1}
	}
	return claims, app, nil
}

func (s *Service) prepareAppPayload(ctx context.Context, name, description string, allowedCaps []string, defaultWorkflows map[string]uint64, callbackWhitelist []string, rateLimit uint32, metaJSON string) (*preparedAppPayload, error) {
	prepared := &preparedAppPayload{
		name:              strings.TrimSpace(name),
		description:       strings.TrimSpace(description),
		allowedCaps:       normalizeCapabilities(allowedCaps),
		defaultWorkflows:  cloneWorkflowMap(defaultWorkflows),
		callbackWhitelist: normalizeStringSlice(callbackWhitelist),
		rateLimit:         rateLimit,
		metaJSON:          strings.TrimSpace(metaJSON),
	}
	if prepared.name == "" {
		return nil, &ValidationError{message: "name is required"}
	}
	if len(prepared.allowedCaps) == 0 {
		return nil, &ValidationError{message: "allowed_caps is required"}
	}
	for _, capability := range prepared.allowedCaps {
		if _, ok := capabilityIndex[capability]; !ok {
			return nil, &ValidationError{message: fmt.Sprintf("unsupported capability: %s", capability)}
		}
	}
	if containsString(prepared.allowedCaps, "skill.invoke") && len(prepared.callbackWhitelist) == 0 {
		return nil, ErrCallbackWhitelistReq
	}
	if prepared.rateLimit == 0 {
		prepared.rateLimit = defaultRateLimitPerSec
	}
	if err := s.validateDefaultWorkflows(ctx, prepared.allowedCaps, prepared.defaultWorkflows); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (s *Service) validateDefaultWorkflows(ctx context.Context, allowedCaps []string, defaultWorkflows map[string]uint64) error {
	if len(defaultWorkflows) == 0 {
		return nil
	}
	for capability, workflowID := range defaultWorkflows {
		if workflowID == 0 {
			return &ValidationError{message: fmt.Sprintf("workflow id for %s must be greater than 0", capability)}
		}
		if !containsString(allowedCaps, capability) {
			return &ValidationError{message: fmt.Sprintf("default workflow capability %s is not enabled for this app", capability)}
		}
		expectedType, ok := workflowTypeForCapability(capability)
		if !ok {
			return &ValidationError{message: fmt.Sprintf("capability %s does not support default workflows", capability)}
		}
		if s.workflowValidator == nil {
			continue
		}
		if _, err := s.workflowValidator.ValidateWorkflowBinding(ctx, workflowID, expectedType); err != nil {
			return &ValidationError{message: err.Error()}
		}
	}
	return nil
}

func (s *Service) signingKey(appID string) []byte {
	mac := hmac.New(sha256.New, []byte(s.platformSecret))
	_, _ = mac.Write([]byte(appID))
	return mac.Sum(nil)
}

type preparedAppPayload struct {
	name              string
	description       string
	allowedCaps       []string
	defaultWorkflows  map[string]uint64
	callbackWhitelist []string
	rateLimit         uint32
	metaJSON          string
}

type appRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
}

type rateBucket struct {
	second int64
	count  uint32
}

func newAppRateLimiter() *appRateLimiter {
	return &appRateLimiter{buckets: make(map[string]*rateBucket)}
}

func (l *appRateLimiter) Allow(appID string, limit uint32) bool {
	if limit == 0 {
		limit = defaultRateLimitPerSec
	}
	now := time.Now().Unix()
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket, ok := l.buckets[appID]
	if !ok {
		bucket = &rateBucket{}
		l.buckets[appID] = bucket
	}
	if bucket.second != now {
		bucket.second = now
		bucket.count = 0
	}
	bucket.count++
	return bucket.count <= limit
}

func workflowTypeForCapability(capability string) (wfdomain.WorkflowType, bool) {
	switch capability {
	case "asr.recognize", "nlp.correct":
		return wfdomain.WorkflowTypeBatch, true
	case "asr.stream":
		return wfdomain.WorkflowTypeRealtime, true
	case "meeting.summary":
		return wfdomain.WorkflowTypeMeeting, true
	case "skill.invoke":
		return wfdomain.WorkflowTypeVoice, true
	default:
		return "", false
	}
}

func toAppResponse(app *openplatformdomain.App) AppResponse {
	allowedCaps := append([]string(nil), app.AllowedCaps...)
	sort.Strings(allowedCaps)
	return AppResponse{
		ID:                app.ID,
		AppID:             app.AppID,
		Name:              app.Name,
		Description:       app.Description,
		SecretHint:        app.SecretHint,
		SecretVersion:     app.SecretVersion,
		Status:            app.Status,
		RateLimitPerSec:   app.RateLimitPerSec,
		AllowedCaps:       allowedCaps,
		DefaultWorkflows:  cloneWorkflowMap(app.DefaultWorkflows),
		CallbackWhitelist: append([]string(nil), app.CallbackWhitelist...),
		CreatedAt:         app.CreatedAt,
		UpdatedAt:         app.UpdatedAt,
	}
}

func normalizeCapabilities(items []string) []string {
	return normalizeStringSlice(items)
}

func normalizeStringSlice(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	sort.Strings(normalized)
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func cloneWorkflowMap(items map[string]uint64) map[string]uint64 {
	if len(items) == 0 {
		return nil
	}
	cloned := make(map[string]uint64, len(items))
	for key, value := range items {
		if strings.TrimSpace(key) == "" || value == 0 {
			continue
		}
		cloned[strings.TrimSpace(key)] = value
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func randomToken(prefix string, rawBytes int) (string, error) {
	b := make([]byte, rawBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func maskSecret(secret string) string {
	trimmed := strings.TrimSpace(secret)
	if len(trimmed) <= 10 {
		return trimmed
	}
	return trimmed[:6] + "****" + trimmed[len(trimmed)-4:]
}

func hashSecret(secret string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(secret), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func verifySecret(secret string, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memory uint32
	var iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	otherHash := argon2.IDKey([]byte(secret), salt, iterations, memory, threads, uint32(len(hash)))
	return subtle.ConstantTimeCompare(hash, otherHash) == 1
}
