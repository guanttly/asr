package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type config struct {
	baseURL       string
	adminUsername string
	adminPassword string
	audioFile     string
	timeout       time.Duration
	skipASRAudio  bool
	fullStream    bool
	keepApps      bool
}

type tester struct {
	cfg         config
	client      *http.Client
	adminToken  string
	cleanup     []func(context.Context)
	callbackURL string
}

type envelope struct {
	Code    any             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type appCredentials struct {
	ID        uint64
	AppID     string
	AppSecret string
}

type requestResult struct {
	Status int
	Code   any
	Body   []byte
	Data   json.RawMessage
}

func main() {
	var cfg config
	flag.StringVar(&cfg.baseURL, "base-url", envOrDefault("OPENAPI_BASE_URL", "http://127.0.0.1:10010"), "gateway base URL")
	flag.StringVar(&cfg.adminUsername, "admin-username", envOrDefault("OPENAPI_ADMIN_USERNAME", "admin"), "admin username")
	flag.StringVar(&cfg.adminPassword, "admin-password", envOrDefault("OPENAPI_ADMIN_PASSWORD", "123456"), "admin password")
	flag.StringVar(&cfg.audioFile, "audio-file", os.Getenv("OPENAPI_AUDIO_FILE"), "optional WAV/MP3 file for ASR tests; a small WAV is generated when empty")
	flag.DurationVar(&cfg.timeout, "timeout", 3*time.Minute, "overall test timeout")
	flag.BoolVar(&cfg.skipASRAudio, "skip-asr-audio", false, "skip real ASR audio endpoints")
	flag.BoolVar(&cfg.fullStream, "full-stream", false, "require the stream upstream and exercise chunk/commit/finish")
	flag.BoolVar(&cfg.keepApps, "keep-apps", false, "keep generated OpenPlatform apps")
	flag.Parse()

	cfg.baseURL = strings.TrimRight(strings.TrimSpace(cfg.baseURL), "/")
	if cfg.baseURL == "" {
		log.Fatal("base-url is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer callbackServer.Close()

	t := &tester{
		cfg:         cfg,
		client:      &http.Client{Timeout: cfg.timeout},
		callbackURL: callbackServer.URL,
	}
	defer t.runCleanup(context.Background())

	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"gateway health", t.checkHealth},
		{"admin login", t.loginAdmin},
		{"openapi auth and app setup", t.checkOpenAuth},
		{"missing token error shape", t.checkMissingToken},
		{"nlp.correct", t.checkNLPCorrect},
		{"meeting summary endpoints", t.checkMeetingSummary},
		{"skill lifecycle", t.checkSkillLifecycle},
		{"asr stream capability", t.checkASRStreamCapability},
	}
	if !cfg.skipASRAudio {
		steps = append(steps, struct {
			name string
			fn   func(context.Context) error
		}{"asr audio endpoints", t.checkASRAudio})
	}

	for _, step := range steps {
		started := time.Now()
		fmt.Printf("RUN  %s\n", step.name)
		if err := step.fn(ctx); err != nil {
			fmt.Printf("FAIL %s (%s)\n", step.name, time.Since(started).Round(time.Millisecond))
			log.Fatal(err)
		}
		fmt.Printf("PASS %s (%s)\n", step.name, time.Since(started).Round(time.Millisecond))
	}
}

func (t *tester) checkHealth(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, t.cfg.baseURL+"/api/health", nil)
	if err != nil {
		return err
	}
	response, err := t.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("GET /api/health returned status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (t *tester) loginAdmin(ctx context.Context) error {
	result, err := t.doJSON(ctx, http.MethodPost, "/api/admin/auth/login", "", map[string]any{
		"username": t.cfg.adminUsername,
		"password": t.cfg.adminPassword,
	})
	if err != nil {
		return err
	}
	if err := result.expectOK(http.StatusOK); err != nil {
		return err
	}
	data, err := decodeObject(result.Data)
	if err != nil {
		return err
	}
	t.adminToken = stringValue(data, "token")
	if t.adminToken == "" {
		return errors.New("admin login response missing data.token")
	}
	return nil
}

func (t *tester) checkOpenAuth(ctx context.Context) error {
	allCaps := []string{"asr.recognize", "asr.stream", "meeting.summary", "nlp.correct", "skill.register", "skill.invoke"}
	app, err := t.createApp(ctx, "openapi-real-all", allCaps, []string{t.callbackURL})
	if err != nil {
		return err
	}
	if _, err := t.issueToken(ctx, app); err != nil {
		return err
	}

	streamApp, err := t.createApp(ctx, "openapi-real-stream", []string{"asr.stream"}, nil)
	if err != nil {
		return err
	}
	if _, err := t.issueToken(ctx, streamApp); err != nil {
		return err
	}
	return nil
}

func (t *tester) checkMissingToken(ctx context.Context) error {
	result, err := t.doJSON(ctx, http.MethodPost, "/openapi/v1/nlp/correct", "", map[string]any{"text": "ping"})
	if err != nil {
		return err
	}
	if result.Status != http.StatusUnauthorized || stringCode(result.Code) != "ERR_OPEN_AUTH_MISSING" {
		return fmt.Errorf("expected 401 ERR_OPEN_AUTH_MISSING, got status=%d code=%v body=%s", result.Status, result.Code, strings.TrimSpace(string(result.Body)))
	}
	return nil
}

func (t *tester) checkNLPCorrect(ctx context.Context) error {
	token, err := t.tokenForCaps(ctx, []string{"nlp.correct"}, nil)
	if err != nil {
		return err
	}
	result, err := t.doJSON(ctx, http.MethodPost, "/openapi/v1/nlp/correct", token, map[string]any{"text": "今天天气不错"})
	if err != nil {
		return err
	}
	if err := result.expectOK(http.StatusOK); err != nil {
		return err
	}
	data, err := decodeObject(result.Data)
	if err != nil {
		return err
	}
	if stringValue(data, "request_id") == "" || stringValue(data, "original_text") == "" || stringValue(data, "corrected_text") == "" {
		return fmt.Errorf("nlp.correct response missing expected fields: %s", strings.TrimSpace(string(result.Body)))
	}
	return nil
}

func (t *tester) checkMeetingSummary(ctx context.Context) error {
	token, err := t.tokenForCaps(ctx, []string{"meeting.summary"}, nil)
	if err != nil {
		return err
	}
	result, err := t.doJSON(ctx, http.MethodGet, "/openapi/v1/meetings/templates", token, nil)
	if err != nil {
		return err
	}
	if err := result.expectOK(http.StatusOK); err != nil {
		return err
	}

	result, err = t.doJSON(ctx, http.MethodPost, "/openapi/v1/meetings/text-summary", token, map[string]any{"text": "今天讨论了 OpenAPI 接入验证，并确认后续修复问题。"})
	if err != nil {
		return err
	}
	if err := result.expectOK(http.StatusOK); err != nil {
		return err
	}
	data, err := decodeObject(result.Data)
	if err != nil {
		return err
	}
	if stringValue(data, "request_id") == "" {
		return fmt.Errorf("meeting text-summary response missing request_id: %s", strings.TrimSpace(string(result.Body)))
	}
	if _, ok := data["summary"].(map[string]any); !ok {
		return fmt.Errorf("meeting text-summary response missing summary: %s", strings.TrimSpace(string(result.Body)))
	}
	return nil
}

func (t *tester) checkSkillLifecycle(ctx context.Context) error {
	token, err := t.tokenForCaps(ctx, []string{"skill.register", "skill.invoke"}, []string{t.callbackURL})
	if err != nil {
		return err
	}
	enabled := true
	name := uniqueName("real_skill")
	result, err := t.doJSON(ctx, http.MethodPost, "/openapi/v1/skills", token, map[string]any{
		"name":                name,
		"display_name":        "Real Skill",
		"description":         "openapi real test skill",
		"intent_patterns":     []string{"re:^turn on (?P<target>.+)$"},
		"parameters":          "{}",
		"callback_url":        t.callbackURL,
		"callback_timeout_ms": 1000,
		"enabled":             enabled,
	})
	if err != nil {
		return err
	}
	if err := result.expectOK(http.StatusOK); err != nil {
		return err
	}
	data, err := decodeObject(result.Data)
	if err != nil {
		return err
	}
	skillID := stringValue(nestedObject(data, "data"), "skill_id")
	if skillID == "" {
		skillID = stringValue(data, "skill_id")
	}
	if skillID == "" {
		return fmt.Errorf("skill create response missing skill_id: %s", strings.TrimSpace(string(result.Body)))
	}

	result, err = t.doJSON(ctx, http.MethodGet, "/openapi/v1/skills", token, nil)
	if err != nil {
		return err
	}
	if err := result.expectOK(http.StatusOK); err != nil {
		return err
	}

	result, err = t.doJSON(ctx, http.MethodPost, "/openapi/v1/skills/"+url.PathEscape(skillID)+"/dry-run", token, map[string]any{"utterance": "turn on light"})
	if err != nil {
		return err
	}
	if err := result.expectOK(http.StatusOK); err != nil {
		return err
	}
	data, err = decodeObject(result.Data)
	if err != nil {
		return err
	}
	dryRunData := nestedObject(data, "data")
	matched, _ := dryRunData["matched"].(bool)
	if !matched {
		return fmt.Errorf("skill dry-run did not match: %s", strings.TrimSpace(string(result.Body)))
	}

	result, err = t.doJSON(ctx, http.MethodDelete, "/openapi/v1/skills/"+url.PathEscape(skillID), token, nil)
	if err != nil {
		return err
	}
	return result.expectOK(http.StatusOK)
}

func (t *tester) checkASRStreamCapability(ctx context.Context) error {
	token, err := t.tokenForCaps(ctx, []string{"asr.stream"}, nil)
	if err != nil {
		return err
	}
	result, err := t.doJSON(ctx, http.MethodPost, "/openapi/v1/asr/stream-sessions", token, nil)
	if err != nil {
		return err
	}
	if result.Status == http.StatusForbidden && stringCode(result.Code) == "ERR_OPEN_CAP_DENIED" {
		return fmt.Errorf("asr.stream token was rejected by stream endpoint capability middleware: %s", strings.TrimSpace(string(result.Body)))
	}
	if !t.cfg.fullStream {
		return nil
	}
	if err := result.expectOK(http.StatusOK); err != nil {
		return err
	}
	data, err := decodeObject(result.Data)
	if err != nil {
		return err
	}
	sessionID := stringValue(data, "session_id")
	if sessionID == "" {
		return fmt.Errorf("stream start response missing session_id: %s", strings.TrimSpace(string(result.Body)))
	}
	if wsURL := stringValue(data, "ws_url"); wsURL != "" {
		if err := t.checkStreamEvents(ctx, wsURL); err != nil {
			return err
		}
	}
	if err := t.pushStreamChunk(ctx, token, sessionID); err != nil {
		return err
	}
	if _, err := t.doJSON(ctx, http.MethodPost, "/openapi/v1/asr/stream-sessions/"+url.PathEscape(sessionID)+"/commit", token, nil); err != nil {
		return err
	}
	finish, err := t.doJSON(ctx, http.MethodPost, "/openapi/v1/asr/stream-sessions/"+url.PathEscape(sessionID)+"/finish", token, nil)
	if err != nil {
		return err
	}
	return finish.expectOK(http.StatusOK)
}

func (t *tester) checkStreamEvents(ctx context.Context, rawURL string) error {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, rawURL, nil)
	if err != nil {
		return fmt.Errorf("dial stream ws_url: %w", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var event map[string]any
	if err := conn.ReadJSON(&event); err != nil {
		return fmt.Errorf("read stream ready event: %w", err)
	}
	if event["type"] != "session.ready" {
		return fmt.Errorf("expected session.ready, got %+v", event)
	}
	return nil
}

func (t *tester) pushStreamChunk(ctx context.Context, token, sessionID string) error {
	pcm := make([]byte, 3200)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.baseURL+"/openapi/v1/asr/stream-sessions/"+url.PathEscape(sessionID)+"/chunks", bytes.NewReader(pcm))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/octet-stream")
	response, err := t.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("push stream chunk returned status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (t *tester) checkASRAudio(ctx context.Context) error {
	token, err := t.tokenForCaps(ctx, []string{"asr.recognize"}, nil)
	if err != nil {
		return err
	}
	audioPath := t.cfg.audioFile
	if strings.TrimSpace(audioPath) == "" {
		tempDir, err := os.MkdirTemp("", "openapi-real-audio-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)
		audioPath = filepath.Join(tempDir, "sample.wav")
		if err := writeSampleWAV(audioPath); err != nil {
			return err
		}
	}

	for _, endpoint := range []string{"/openapi/v1/asr/recognize", "/openapi/v1/asr/recognize/vad", "/openapi/v1/asr/tasks"} {
		result, err := t.doMultipart(ctx, endpoint, token, "file", audioPath, map[string]string{"language": "zh-CN", "use_itn": "true"})
		if err != nil {
			return err
		}
		if err := result.expectOK(http.StatusOK); err != nil {
			return fmt.Errorf("%s failed: %w", endpoint, err)
		}
		data, err := decodeObject(result.Data)
		if err != nil {
			return err
		}
		taskID := stringValue(data, "task_id")
		if taskID == "" {
			return fmt.Errorf("%s response missing task_id: %s", endpoint, strings.TrimSpace(string(result.Body)))
		}
		if endpoint == "/openapi/v1/asr/tasks" {
			if err := t.checkTask(ctx, token, taskID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *tester) checkTask(ctx context.Context, token, taskID string) error {
	result, err := t.doJSON(ctx, http.MethodGet, "/openapi/v1/asr/tasks/"+url.PathEscape(taskID), token, nil)
	if err != nil {
		return err
	}
	if err := result.expectOK(http.StatusOK); err != nil {
		return err
	}
	data, err := decodeObject(result.Data)
	if err != nil {
		return err
	}
	if stringValue(data, "task_id") == "" || stringValue(data, "status") == "" {
		return fmt.Errorf("task query response missing task_id/status: %s", strings.TrimSpace(string(result.Body)))
	}
	return nil
}

func (t *tester) tokenForCaps(ctx context.Context, caps []string, whitelist []string) (string, error) {
	app, err := t.createApp(ctx, "openapi-real", caps, whitelist)
	if err != nil {
		return "", err
	}
	return t.issueToken(ctx, app)
}

func (t *tester) createApp(ctx context.Context, prefix string, caps []string, whitelist []string) (*appCredentials, error) {
	result, err := t.doJSON(ctx, http.MethodPost, "/api/admin/openplatform/apps", t.adminToken, map[string]any{
		"name":               uniqueName(prefix),
		"description":        "generated by openapi-real-test",
		"allowed_caps":       caps,
		"callback_whitelist": whitelist,
		"rate_limit_per_sec": 100,
		"default_workflows":  map[string]uint64{},
		"meta_json":          "{}",
	})
	if err != nil {
		return nil, err
	}
	if err := result.expectOK(http.StatusOK); err != nil {
		return nil, err
	}
	data, err := decodeObject(result.Data)
	if err != nil {
		return nil, err
	}
	app := &appCredentials{
		ID:        uint64(numberValue(data, "id")),
		AppID:     stringValue(data, "app_id"),
		AppSecret: stringValue(data, "app_secret"),
	}
	if app.ID == 0 || app.AppID == "" || app.AppSecret == "" {
		return nil, fmt.Errorf("create app response missing id/app_id/app_secret: %s", strings.TrimSpace(string(result.Body)))
	}
	if !t.cfg.keepApps {
		t.cleanup = append(t.cleanup, func(ctx context.Context) {
			_, _ = t.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/api/admin/openplatform/apps/%d", app.ID), t.adminToken, nil)
		})
	}
	return app, nil
}

func (t *tester) issueToken(ctx context.Context, app *appCredentials) (string, error) {
	result, err := t.doJSON(ctx, http.MethodPost, "/openapi/v1/auth/token", "", map[string]any{
		"app_id":     app.AppID,
		"app_secret": app.AppSecret,
	})
	if err != nil {
		return "", err
	}
	if err := result.expectOK(http.StatusOK); err != nil {
		return "", err
	}
	data, err := decodeObject(result.Data)
	if err != nil {
		return "", err
	}
	token := stringValue(data, "access_token")
	if token == "" {
		return "", fmt.Errorf("token response missing access_token: %s", strings.TrimSpace(string(result.Body)))
	}
	return token, nil
}

func (t *tester) doJSON(ctx context.Context, method, pathValue, bearerToken string, payload any) (*requestResult, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, t.cfg.baseURL+pathValue, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	return t.do(request)
}

func (t *tester) doMultipart(ctx context.Context, pathValue, bearerToken, fileField, filePath string, fields map[string]string) (*requestResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fileField, filepath.Base(filePath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.baseURL+pathValue, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	return t.do(request)
}

func (t *tester) do(request *http.Request) (*requestResult, error) {
	response, err := t.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	var env envelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&env); err != nil {
		return nil, fmt.Errorf("decode %s %s response: %w; body=%s", request.Method, request.URL.Path, err, strings.TrimSpace(string(body)))
	}
	return &requestResult{Status: response.StatusCode, Code: env.Code, Body: body, Data: env.Data}, nil
}

func (r *requestResult) expectOK(status int) error {
	if r.Status != status || !isCodeOK(r.Code) {
		return fmt.Errorf("expected status=%d code=0, got status=%d code=%v body=%s", status, r.Status, r.Code, strings.TrimSpace(string(r.Body)))
	}
	return nil
}

func (t *tester) runCleanup(ctx context.Context) {
	for i := len(t.cleanup) - 1; i >= 0; i-- {
		t.cleanup[i](ctx)
	}
}

func writeSampleWAV(path string) error {
	const sampleRate = 16000
	const seconds = 1
	samples := sampleRate * seconds
	pcm := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		value := int16(math.Sin(2*math.Pi*440*float64(i)/sampleRate) * 12000)
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(value))
	}
	return writePCM16MonoWAV(path, pcm, sampleRate)
}

func writePCM16MonoWAV(outputPath string, pcmData []byte, sampleRate int) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	dataSize := len(pcmData)
	chunkSize := 36 + dataSize
	if _, err := file.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(chunkSize)); err != nil {
		return err
	}
	if _, err := file.Write([]byte("WAVEfmt ")); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(sampleRate*2)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(2)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(16)); err != nil {
		return err
	}
	if _, err := file.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(dataSize)); err != nil {
		return err
	}
	_, err = file.Write(pcmData)
	return err
}

func decodeObject(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func nestedObject(raw map[string]any, key string) map[string]any {
	value, _ := raw[key].(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}

func stringValue(raw map[string]any, key string) string {
	value, ok := raw[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func numberValue(raw map[string]any, key string) int64 {
	value, ok := raw[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	default:
		return 0
	}
}

func isCodeOK(code any) bool {
	switch typed := code.(type) {
	case json.Number:
		return typed.String() == "0"
	case float64:
		return typed == 0
	case string:
		return typed == "0"
	case nil:
		return false
	default:
		return fmt.Sprint(typed) == "0"
	}
}

func stringCode(code any) string {
	switch typed := code.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func uniqueName(prefix string) string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%d-%x", prefix, time.Now().Unix(), buf)
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
