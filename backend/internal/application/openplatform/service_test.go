package openplatform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
)

type openAppRepoStub struct {
	items  map[uint64]*openplatformdomain.App
	byApp  map[string]uint64
	nextID uint64
}

func newOpenAppRepoStub() *openAppRepoStub {
	return &openAppRepoStub{items: map[uint64]*openplatformdomain.App{}, byApp: map[string]uint64{}, nextID: 1}
}

func (r *openAppRepoStub) Create(_ context.Context, app *openplatformdomain.App) error {
	for _, item := range r.items {
		if item.Name == app.Name {
			return openplatformdomain.ErrAppAlreadyExists
		}
	}
	copy := *app
	copy.ID = r.nextID
	r.nextID++
	r.items[copy.ID] = &copy
	r.byApp[copy.AppID] = copy.ID
	app.ID = copy.ID
	return nil
}

func (r *openAppRepoStub) GetByID(_ context.Context, id uint64) (*openplatformdomain.App, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, openplatformdomain.ErrAppNotFound
	}
	copy := *item
	return &copy, nil
}

func (r *openAppRepoStub) GetByAppID(_ context.Context, appID string) (*openplatformdomain.App, error) {
	id, ok := r.byApp[appID]
	if !ok {
		return nil, openplatformdomain.ErrAppNotFound
	}
	return r.GetByID(context.Background(), id)
}

func (r *openAppRepoStub) GetByName(_ context.Context, name string) (*openplatformdomain.App, error) {
	for _, item := range r.items {
		if item.Name == name {
			copy := *item
			return &copy, nil
		}
	}
	return nil, openplatformdomain.ErrAppNotFound
}

func (r *openAppRepoStub) Update(_ context.Context, app *openplatformdomain.App) error {
	if _, ok := r.items[app.ID]; !ok {
		return openplatformdomain.ErrAppNotFound
	}
	copy := *app
	r.items[app.ID] = &copy
	r.byApp[app.AppID] = app.ID
	return nil
}

func (r *openAppRepoStub) UpdateStatus(_ context.Context, id uint64, status openplatformdomain.AppStatus) error {
	item, ok := r.items[id]
	if !ok {
		return openplatformdomain.ErrAppNotFound
	}
	item.Status = status
	return nil
}

func (r *openAppRepoStub) List(_ context.Context, offset, limit int) ([]*openplatformdomain.App, int64, error) {
	items := make([]*openplatformdomain.App, 0, len(r.items))
	for _, item := range r.items {
		copy := *item
		items = append(items, &copy)
	}
	return items, int64(len(items)), nil
}

type noopSkillRepo struct{}

func (noopSkillRepo) Create(context.Context, *openplatformdomain.Skill) error { return nil }
func (noopSkillRepo) GetByID(context.Context, uint64) (*openplatformdomain.Skill, error) {
	return nil, openplatformdomain.ErrSkillNotFound
}
func (noopSkillRepo) GetByUID(context.Context, string) (*openplatformdomain.Skill, error) {
	return nil, openplatformdomain.ErrSkillNotFound
}
func (noopSkillRepo) GetByAppAndName(context.Context, uint64, string) (*openplatformdomain.Skill, error) {
	return nil, openplatformdomain.ErrSkillNotFound
}
func (noopSkillRepo) Update(context.Context, *openplatformdomain.Skill) error { return nil }
func (noopSkillRepo) Delete(context.Context, uint64) error                    { return nil }
func (noopSkillRepo) ListByApp(context.Context, uint64) ([]*openplatformdomain.Skill, error) {
	return nil, nil
}

type skillRepoStub struct {
	items  map[uint64]*openplatformdomain.Skill
	byUID  map[string]uint64
	nextID uint64
}

func newSkillRepoStub() *skillRepoStub {
	return &skillRepoStub{items: map[uint64]*openplatformdomain.Skill{}, byUID: map[string]uint64{}, nextID: 1}
}

func (r *skillRepoStub) Create(_ context.Context, skill *openplatformdomain.Skill) error {
	for _, item := range r.items {
		if item.AppID == skill.AppID && item.Name == skill.Name {
			return openplatformdomain.ErrSkillNameDuplicated
		}
	}
	copy := *skill
	copy.ID = r.nextID
	r.nextID++
	r.items[copy.ID] = &copy
	r.byUID[copy.SkillUID] = copy.ID
	skill.ID = copy.ID
	return nil
}

func (r *skillRepoStub) GetByID(_ context.Context, id uint64) (*openplatformdomain.Skill, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, openplatformdomain.ErrSkillNotFound
	}
	copy := *item
	return &copy, nil
}

func (r *skillRepoStub) GetByUID(_ context.Context, uid string) (*openplatformdomain.Skill, error) {
	id, ok := r.byUID[uid]
	if !ok {
		return nil, openplatformdomain.ErrSkillNotFound
	}
	return r.GetByID(context.Background(), id)
}

func (r *skillRepoStub) GetByAppAndName(_ context.Context, appID uint64, name string) (*openplatformdomain.Skill, error) {
	for _, item := range r.items {
		if item.AppID == appID && item.Name == name {
			copy := *item
			return &copy, nil
		}
	}
	return nil, openplatformdomain.ErrSkillNotFound
}

func (r *skillRepoStub) Update(_ context.Context, skill *openplatformdomain.Skill) error {
	if _, ok := r.items[skill.ID]; !ok {
		return openplatformdomain.ErrSkillNotFound
	}
	copy := *skill
	r.items[skill.ID] = &copy
	r.byUID[skill.SkillUID] = skill.ID
	return nil
}

func (r *skillRepoStub) Delete(_ context.Context, id uint64) error {
	item, ok := r.items[id]
	if !ok {
		return openplatformdomain.ErrSkillNotFound
	}
	delete(r.byUID, item.SkillUID)
	delete(r.items, id)
	return nil
}

func (r *skillRepoStub) ListByApp(_ context.Context, appID uint64) ([]*openplatformdomain.Skill, error) {
	items := make([]*openplatformdomain.Skill, 0)
	for _, item := range r.items {
		if item.AppID != appID {
			continue
		}
		copy := *item
		items = append(items, &copy)
	}
	return items, nil
}

type noopCallLogRepo struct{}

func (noopCallLogRepo) Create(context.Context, *openplatformdomain.CallLog) error { return nil }
func (noopCallLogRepo) ListByApp(context.Context, uint64, int) ([]*openplatformdomain.CallLog, error) {
	return nil, nil
}

type skillInvocationRepoStub struct {
	items []*openplatformdomain.SkillInvocation
}

func (r *skillInvocationRepoStub) Create(_ context.Context, invocation *openplatformdomain.SkillInvocation) error {
	copy := *invocation
	r.items = append(r.items, &copy)
	return nil
}

func (r *skillInvocationRepoStub) ListBySkill(_ context.Context, skillID uint64, _ int) ([]*openplatformdomain.SkillInvocation, error) {
	items := make([]*openplatformdomain.SkillInvocation, 0)
	for _, item := range r.items {
		if item.SkillID != skillID {
			continue
		}
		copy := *item
		items = append(items, &copy)
	}
	return items, nil
}

func TestCreateAppIssueAndAuthenticateToken(t *testing.T) {
	repo := newOpenAppRepoStub()
	service := NewService(repo, noopSkillRepo{}, noopCallLogRepo{}, nil, "unit-test-secret", 3600)

	created, err := service.CreateApp(context.Background(), 7, &CreateAppRequest{
		Name:        "客服助手",
		AllowedCaps: []string{"asr.recognize", "meeting.summary"},
	})
	if err != nil {
		t.Fatalf("CreateApp returned error: %v", err)
	}
	if created.AppSecret == "" {
		t.Fatal("expected app_secret to be returned once")
	}
	if created.SecretHint == "" {
		t.Fatal("expected secret hint to be stored")
	}

	token, err := service.IssueToken(context.Background(), &IssueTokenRequest{AppID: created.AppID, AppSecret: created.AppSecret})
	if err != nil {
		t.Fatalf("IssueToken returned error: %v", err)
	}
	if token.AccessToken == "" {
		t.Fatal("expected access token")
	}

	claims, app, err := service.AuthenticateAccessToken(context.Background(), token.AccessToken, "asr.recognize")
	if err != nil {
		t.Fatalf("AuthenticateAccessToken returned error: %v", err)
	}
	if app.AppID != created.AppID {
		t.Fatalf("expected app_id %s, got %s", created.AppID, app.AppID)
	}
	if claims.AppID != created.AppID {
		t.Fatalf("expected claims app_id %s, got %s", created.AppID, claims.AppID)
	}
}

func TestCreateAppRequiresCallbackWhitelistWhenSkillInvokeEnabled(t *testing.T) {
	repo := newOpenAppRepoStub()
	service := NewService(repo, noopSkillRepo{}, noopCallLogRepo{}, nil, "unit-test-secret", 3600)

	_, err := service.CreateApp(context.Background(), 7, &CreateAppRequest{
		Name:        "技能回调应用",
		AllowedCaps: []string{"skill.invoke"},
	})
	if err == nil {
		t.Fatal("expected callback whitelist validation error")
	}
	if err != ErrCallbackWhitelistReq {
		t.Fatalf("expected ErrCallbackWhitelistReq, got %v", err)
	}
}

func TestAuthenticateRejectsCapabilityOutsideGrant(t *testing.T) {
	repo := newOpenAppRepoStub()
	service := NewService(repo, noopSkillRepo{}, noopCallLogRepo{}, nil, "unit-test-secret", 3600)

	created, err := service.CreateApp(context.Background(), 7, &CreateAppRequest{
		Name:        "只读纠错",
		AllowedCaps: []string{"nlp.correct"},
	})
	if err != nil {
		t.Fatalf("CreateApp returned error: %v", err)
	}
	token, err := service.IssueToken(context.Background(), &IssueTokenRequest{AppID: created.AppID, AppSecret: created.AppSecret})
	if err != nil {
		t.Fatalf("IssueToken returned error: %v", err)
	}

	_, _, err = service.AuthenticateAccessToken(context.Background(), token.AccessToken, "meeting.summary")
	if err != ErrOpenCapDenied {
		t.Fatalf("expected ErrOpenCapDenied, got %v", err)
	}
}

func TestCreateSkillAndDryRun(t *testing.T) {
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer callbackServer.Close()

	appRepo := newOpenAppRepoStub()
	skillRepo := newSkillRepoStub()
	service := NewService(appRepo, skillRepo, noopCallLogRepo{}, nil, "unit-test-secret", 3600)

	created, err := service.CreateApp(context.Background(), 7, &CreateAppRequest{
		Name:              "会议技能应用",
		AllowedCaps:       []string{"skill.register", "skill.invoke"},
		CallbackWhitelist: []string{callbackServer.URL},
	})
	if err != nil {
		t.Fatalf("CreateApp returned error: %v", err)
	}
	app, err := appRepo.GetByAppID(context.Background(), created.AppID)
	if err != nil {
		t.Fatalf("GetByAppID returned error: %v", err)
	}

	skill, err := service.CreateSkill(context.Background(), app, &CreateSkillRequest{
		Name:           "book_meeting_room",
		DisplayName:    "预订会议室",
		IntentPatterns: []string{"预订会议室", "帮我订会议室"},
		CallbackURL:    callbackServer.URL + "/skills/book-room",
	})
	if err != nil {
		t.Fatalf("CreateSkill returned error: %v", err)
	}
	if skill.SkillID == "" {
		t.Fatal("expected skill_id")
	}

	dryRun, err := service.DryRunSkill(context.Background(), app, skill.SkillID, "请帮我预订会议室")
	if err != nil {
		t.Fatalf("DryRunSkill returned error: %v", err)
	}
	if !dryRun.Matched {
		t.Fatal("expected dry-run to match pattern")
	}
	if dryRun.MatchedPattern == "" {
		t.Fatal("expected matched pattern")
	}
}

func TestDispatchOpenCallbackSignsWithStoredSecret(t *testing.T) {
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read callback body: %v", err)
		}
		mac := hmac.New(sha256.New, []byte("unit-test-app-secret"))
		_, _ = mac.Write(payload)
		expected := "hmac-sha256=" + hex.EncodeToString(mac.Sum(nil))
		if r.Header.Get("X-OpenAPI-Signature") != expected {
			t.Fatalf("unexpected signature header: %s", r.Header.Get("X-OpenAPI-Signature"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer callbackServer.Close()

	service := NewService(newOpenAppRepoStub(), noopSkillRepo{}, noopCallLogRepo{}, nil, "unit-test-secret", 3600)
	service.callbackRetryBackoffs = []time.Duration{0}
	app := &openplatformdomain.App{AppSecretCiphertext: mustEncryptSecretForTest(t, service, "unit-test-app-secret")}
	_, status, err := service.DispatchOpenCallback(context.Background(), app, callbackServer.URL, map[string]any{"ok": true}, nil)
	if err != nil {
		t.Fatalf("DispatchOpenCallback returned error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}
}

func TestMatchAndInvokeSkillRecordsInvocationAndDisablesAfterFailures(t *testing.T) {
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"accepted":false}`))
	}))
	defer callbackServer.Close()

	appRepo := newOpenAppRepoStub()
	skillRepo := newSkillRepoStub()
	invocationRepo := &skillInvocationRepoStub{}
	service := NewService(appRepo, skillRepo, noopCallLogRepo{}, nil, "unit-test-secret", 3600)
	service.SetSkillInvocationRepo(invocationRepo)
	service.callbackRetryBackoffs = []time.Duration{0}

	created, err := service.CreateApp(context.Background(), 7, &CreateAppRequest{
		Name:              "语音技能应用",
		AllowedCaps:       []string{"skill.register", "skill.invoke"},
		CallbackWhitelist: []string{callbackServer.URL},
	})
	if err != nil {
		t.Fatalf("CreateApp returned error: %v", err)
	}
	app, err := appRepo.GetByAppID(context.Background(), created.AppID)
	if err != nil {
		t.Fatalf("GetByAppID returned error: %v", err)
	}
	skill, err := service.CreateSkill(context.Background(), app, &CreateSkillRequest{
		Name:           "book_meeting_room",
		DisplayName:    "预订会议室",
		IntentPatterns: []string{"预订会议室", "帮我订会议室"},
		CallbackURL:    callbackServer.URL + "/invoke",
	})
	if err != nil {
		t.Fatalf("CreateSkill returned error: %v", err)
	}
	storedSkill, err := skillRepo.GetByUID(context.Background(), skill.SkillID)
	if err != nil {
		t.Fatalf("GetByUID returned error: %v", err)
	}

	for attempt := 1; attempt <= 5; attempt++ {
		result, invokeErr := service.MatchAndInvokeSkill(context.Background(), openplatformdomain.OwnerUserIDForApp(app.ID), "请帮我预订会议室", 11, 0)
		if invokeErr != nil {
			t.Fatalf("MatchAndInvokeSkill attempt %d returned error: %v", attempt, invokeErr)
		}
		if result == nil || result.Status != openplatformdomain.InvocationStatusFailed {
			t.Fatalf("expected failed invocation result on attempt %d, got %+v", attempt, result)
		}
	}

	updatedSkill, err := skillRepo.GetByUID(context.Background(), skill.SkillID)
	if err != nil {
		t.Fatalf("GetByUID after invoke returned error: %v", err)
	}
	if updatedSkill.Enabled {
		t.Fatal("expected skill to be auto-disabled after consecutive failures")
	}
	if updatedSkill.ConsecutiveFailures != 5 {
		t.Fatalf("expected 5 consecutive failures, got %d", updatedSkill.ConsecutiveFailures)
	}
	items, err := invocationRepo.ListBySkill(context.Background(), storedSkill.ID, 10)
	if err != nil {
		t.Fatalf("ListBySkill returned error: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("expected 5 invocation records, got %d", len(items))
	}
	if items[0].HTTPStatus == nil || *items[0].HTTPStatus != http.StatusBadGateway {
		t.Fatalf("expected invocation http status 502, got %+v", items[0].HTTPStatus)
	}
	if items[0].Status != openplatformdomain.InvocationStatusFailed {
		t.Fatalf("expected failed invocation status, got %s", items[0].Status)
	}
}

func mustEncryptSecretForTest(t *testing.T, service *Service, secret string) string {
	t.Helper()
	value, err := service.encryptSecret(secret)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	return value
}
