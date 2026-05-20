package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	appasr "github.com/lgt/asr/internal/application/asr"
)

func TestLegacyRecognizeMatchesOldHTTPShapeAndForwardsOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	batchEngine := &batchEngineHandlerStub{submitResult: &appasr.BatchSubmitResult{Status: "completed", ResultText: "识别完成", Duration: 1.25}}
	handler := NewLegacyASRHandler(appasr.NewService(nil, batchEngine, nil, 5, nil), nil, t.TempDir(), "http://uploads.example.test", 100)
	router := gin.New()
	handler.Register(router.Group("/api"))

	body, contentType := newLegacyMultipartBody(t, "file", "sample.wav", map[string]string{
		"language": "zh-CN",
		"use_itn":  "false",
		"hotwords": "术语A,术语B;术语C\n术语D",
	})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/recognize", body)
	req.Header.Set("Content-Type", contentType)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["success"] != true {
		t.Fatalf("expected success=true, got %+v", response)
	}
	if response["filename"] != "sample.wav" {
		t.Fatalf("expected old top-level filename, got %+v", response)
	}
	if response["language"] != "zh-CN" {
		t.Fatalf("expected old top-level language, got %+v", response)
	}
	if response["use_vad_segmentation"] != false {
		t.Fatalf("expected use_vad_segmentation=false, got %+v", response)
	}
	result, ok := response["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected old top-level result object, got %+v", response)
	}
	if result["text"] != "识别完成" {
		t.Fatalf("expected result text, got %+v", result)
	}
	if response["data"] == nil {
		t.Fatalf("expected data to remain available for current legacy clients, got %+v", response)
	}
	if batchEngine.lastReq.Language != "zh" {
		t.Fatalf("expected old zh-CN language mapped for ASR engine, got %q", batchEngine.lastReq.Language)
	}
	if batchEngine.lastReq.UseITN == nil || *batchEngine.lastReq.UseITN {
		t.Fatalf("expected use_itn=false forwarded to ASR engine, got %#v", batchEngine.lastReq.UseITN)
	}
	expectedHotwords := []string{"术语A", "术语B", "术语C", "术语D"}
	if strings.Join(batchEngine.lastReq.Hotwords, ",") != strings.Join(expectedHotwords, ",") {
		t.Fatalf("expected hotwords %v, got %v", expectedHotwords, batchEngine.lastReq.Hotwords)
	}
}

func TestLegacyRecognizeVADReturnsOldSegmentsShape(t *testing.T) {
	gin.SetMode(gin.TestMode)

	batchEngine := &batchEngineHandlerStub{submitResult: &appasr.BatchSubmitResult{Status: "completed", ResultText: "分段文本", Duration: 2}}
	handler := NewLegacyASRHandler(appasr.NewService(nil, batchEngine, nil, 5, nil), nil, t.TempDir(), "http://uploads.example.test", 100)
	router := gin.New()
	handler.Register(router.Group("/api"))

	body, contentType := newLegacyMultipartBody(t, "file", "sample.wav", map[string]string{
		"min_segment_duration": "1",
		"max_segment_duration": "30",
	})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/recognize/vad", body)
	req.Header.Set("Content-Type", contentType)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["use_vad_segmentation"] != true {
		t.Fatalf("expected use_vad_segmentation=true, got %+v", response)
	}
	result := response["result"].(map[string]any)
	segments, ok := result["segments"].([]any)
	if !ok || len(segments) != 1 {
		t.Fatalf("expected one VAD segment, got %+v", result)
	}
}

func TestLegacyAudioToSummaryCallbackUsesOldContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	callbackBodies := make(chan []byte, 1)
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-ASR-Signature") != "" {
			t.Errorf("legacy callback must not include HMAC signature headers")
		}
		body, _ := io.ReadAll(r.Body)
		callbackBodies <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer callbackServer.Close()

	batchEngine := &batchEngineHandlerStub{submitResult: &appasr.BatchSubmitResult{Status: "completed", ResultText: "会议内容", Duration: 3}}
	repo := &taskRepoHandlerStub{}
	handler := NewLegacyASRHandler(appasr.NewService(repo, batchEngine, nil, 5, nil), nil, t.TempDir(), "http://uploads.example.test", 100)
	router := gin.New()
	handler.Register(router.Group("/api"))

	body, contentType := newLegacyMultipartBody(t, "audio_file", "meeting.wav", map[string]string{
		"template_name":     "default",
		"enable_correction": "true",
		"enable_speaker":    "true",
		"language":          "en-US",
		"use_itn":           "false",
		"callback":          callbackServer.URL,
	})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/audio/to_summary", body)
	req.Header.Set("Content-Type", contentType)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response legacyEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode immediate response: %v", err)
	}
	if !response.Success || !strings.Contains(response.Message, "回调") {
		t.Fatalf("expected old callback task creation response, got %+v", response)
	}

	select {
	case body := <-callbackBodies:
		var callback map[string]any
		if err := json.Unmarshal(body, &callback); err != nil {
			t.Fatalf("decode callback body: %v", err)
		}
		if callback["success"] != true {
			t.Fatalf("expected callback success=true, got %+v", callback)
		}
		data := callback["data"].(map[string]any)
		asrResult := data["asr_result"].(map[string]any)
		if asrResult["text"] != "会议内容" {
			t.Fatalf("expected callback ASR text, got %+v", data)
		}
		processingInfo := data["processing_info"].(map[string]any)
		if processingInfo["template_used"] != "default" || processingInfo["correction_enabled"] != true || processingInfo["speaker_identification_enabled"] != true {
			t.Fatalf("expected old processing_info fields, got %+v", processingInfo)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for legacy callback")
	}

	if batchEngine.lastReq.Language != "en" {
		t.Fatalf("expected old en-US language mapped for ASR engine, got %q", batchEngine.lastReq.Language)
	}
	if batchEngine.lastReq.UseITN == nil || *batchEngine.lastReq.UseITN {
		t.Fatalf("expected audio summary use_itn=false forwarded, got %#v", batchEngine.lastReq.UseITN)
	}
}

func TestLegacyAudioToSummaryRejectsInvalidCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	batchEngine := &batchEngineHandlerStub{submitResult: &appasr.BatchSubmitResult{Status: "completed", ResultText: "会议内容", Duration: 3}}
	repo := &taskRepoHandlerStub{}
	handler := NewLegacyASRHandler(appasr.NewService(repo, batchEngine, nil, 5, nil), nil, t.TempDir(), "http://uploads.example.test", 100)
	router := gin.New()
	handler.Register(router.Group("/api"))

	body, contentType := newLegacyMultipartBody(t, "audio_file", "meeting.wav", map[string]string{
		"callback": "ftp://example.com/callback",
	})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/audio/to_summary", body)
	req.Header.Set("Content-Type", contentType)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(repo.tasks) != 0 {
		t.Fatalf("expected invalid callback to be rejected before task creation, got %d tasks", len(repo.tasks))
	}
}

func newLegacyMultipartBody(t *testing.T, fileField, filename string, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fileField, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("not a real wav, but enough for handler compatibility tests")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body, writer.FormDataContentType()
}
