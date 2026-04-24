package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	appvoiceprint "github.com/lgt/asr/internal/application/voiceprint"
	"github.com/lgt/asr/internal/infrastructure/diarization"
	pkgconfig "github.com/lgt/asr/pkg/config"
)

type voiceprintClientStub struct {
	baseURL       string
	listRecords   []diarization.VoiceprintRecord
	enrollRecord  *diarization.VoiceprintRecord
	listErr       error
	enrollErr     error
	deleteErr     error
	deletedID     string
	lastAudioPath string
	lastMetadata  diarization.VoiceprintMetadata
}

func (s *voiceprintClientStub) BaseURL() string {
	return s.baseURL
}

func (s *voiceprintClientStub) ListVoiceprints(_ context.Context) ([]diarization.VoiceprintRecord, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]diarization.VoiceprintRecord(nil), s.listRecords...), nil
}

func (s *voiceprintClientStub) EnrollVoiceprint(_ context.Context, audioFilePath string, metadata diarization.VoiceprintMetadata) (*diarization.VoiceprintRecord, error) {
	s.lastAudioPath = audioFilePath
	s.lastMetadata = metadata
	if s.enrollErr != nil {
		return nil, s.enrollErr
	}
	if s.enrollRecord == nil {
		return &diarization.VoiceprintRecord{}, nil
	}
	copy := *s.enrollRecord
	return &copy, nil
}

func (s *voiceprintClientStub) DeleteVoiceprint(_ context.Context, recordID string) error {
	s.deletedID = recordID
	return s.deleteErr
}

func TestVoiceprintHandlerList(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	handler := NewVoiceprintHandler(appvoiceprint.NewService(&voiceprintClientStub{
		baseURL: "http://speaker-analysis:8100",
		listRecords: []diarization.VoiceprintRecord{{
			ID:            "vp-1",
			SpeakerName:   "张三",
			Department:    "产品部",
			Notes:         "主讲人",
			AudioDuration: 16.2,
			CreatedAt:     "2025-04-10T08:30:00",
			UpdatedAt:     "2025-04-10T08:30:00",
		}},
	}), 100, pkgconfig.ProductConfig{Edition: pkgconfig.ProductEditionAdvanced}.Features())

	router := gin.New()
	router.GET("/voiceprints", handler.List)

	req := httptest.NewRequest(http.MethodGet, "/voiceprints", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var envelope responseEnvelope[struct {
		Items      []appvoiceprint.Record `json:"items"`
		Total      int                    `json:"total"`
		ServiceURL string                 `json:"service_url"`
	}]
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Data.Total != 1 {
		t.Fatalf("expected total 1, got %d", envelope.Data.Total)
	}
	if len(envelope.Data.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(envelope.Data.Items))
	}
	if envelope.Data.Items[0].SpeakerName != "张三" {
		t.Fatalf("expected speaker_name 张三, got %q", envelope.Data.Items[0].SpeakerName)
	}
	if envelope.Data.ServiceURL != "http://speaker-analysis:8100" {
		t.Fatalf("expected service url returned, got %q", envelope.Data.ServiceURL)
	}
}

func TestVoiceprintHandlerEnroll(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	client := &voiceprintClientStub{
		baseURL: "http://speaker-analysis:8100",
		enrollRecord: &diarization.VoiceprintRecord{
			ID:            "vp-2",
			SpeakerName:   "李四",
			Department:    "研发部",
			Notes:         "主讲人样本",
			AudioDuration: 21.8,
			CreatedAt:     "2025-04-10T09:00:00",
			UpdatedAt:     "2025-04-10T09:00:00",
		},
	}
	handler := NewVoiceprintHandler(appvoiceprint.NewService(client), 100, pkgconfig.ProductConfig{Edition: pkgconfig.ProductEditionAdvanced}.Features())

	router := gin.New()
	router.POST("/voiceprints", handler.Enroll)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileWriter, err := writer.CreateFormFile("file", "sample.wav")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fileWriter.Write([]byte("fake wav data")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.WriteField("speaker_name", "李四"); err != nil {
		t.Fatalf("write speaker_name: %v", err)
	}
	if err := writer.WriteField("department", "研发部"); err != nil {
		t.Fatalf("write department: %v", err)
	}
	if err := writer.WriteField("notes", "主讲人样本"); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/voiceprints", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if client.lastAudioPath == "" {
		t.Fatal("expected uploaded audio path passed to service")
	}
	if client.lastMetadata.SpeakerName != "李四" {
		t.Fatalf("expected speaker_name 李四, got %q", client.lastMetadata.SpeakerName)
	}

	var envelope responseEnvelope[struct {
		Record     appvoiceprint.Record `json:"record"`
		ServiceURL string               `json:"service_url"`
	}]
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Data.Record.ID != "vp-2" {
		t.Fatalf("expected record id vp-2, got %q", envelope.Data.Record.ID)
	}
	if envelope.Data.ServiceURL != "http://speaker-analysis:8100" {
		t.Fatalf("expected service url returned, got %q", envelope.Data.ServiceURL)
	}
}

func TestVoiceprintHandlerReturnsServiceUnavailableWhenUnconfigured(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	handler := NewVoiceprintHandler(appvoiceprint.NewService(&voiceprintClientStub{}), 100, pkgconfig.ProductConfig{Edition: pkgconfig.ProductEditionAdvanced}.Features())

	router := gin.New()
	router.GET("/voiceprints", handler.List)

	req := httptest.NewRequest(http.MethodGet, "/voiceprints", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}
