package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	appmeeting "github.com/lgt/asr/internal/application/meeting"
	appwf "github.com/lgt/asr/internal/application/workflow"
	meetingdomain "github.com/lgt/asr/internal/domain/meeting"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
	"github.com/lgt/asr/internal/interfaces/middleware"
	pkgconfig "github.com/lgt/asr/pkg/config"
	"gorm.io/gorm"
)

type meetingRepoHandlerStub struct {
	meeting *meetingdomain.Meeting
	deleted uint64
}

func (s *meetingRepoHandlerStub) Create(_ context.Context, meeting *meetingdomain.Meeting) error {
	s.meeting = meeting
	return nil
}

func (s *meetingRepoHandlerStub) GetByID(_ context.Context, id uint64) (*meetingdomain.Meeting, error) {
	if s.meeting == nil || s.meeting.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *s.meeting
	return &copy, nil
}

func (s *meetingRepoHandlerStub) GetBySourceTaskID(_ context.Context, _ uint64) (*meetingdomain.Meeting, error) {
	return nil, nil
}

func (s *meetingRepoHandlerStub) Update(_ context.Context, meeting *meetingdomain.Meeting) error {
	s.meeting = meeting
	return nil
}

func (s *meetingRepoHandlerStub) List(_ context.Context, _ uint64, _, _ int) ([]*meetingdomain.Meeting, int64, error) {
	return nil, 0, nil
}

func (s *meetingRepoHandlerStub) ListSyncCandidates(_ context.Context, _ int) ([]*meetingdomain.Meeting, error) {
	return nil, nil
}

func (s *meetingRepoHandlerStub) Delete(_ context.Context, id uint64) error {
	s.deleted = id
	return nil
}

type transcriptRepoHandlerStub struct{}

func (s *transcriptRepoHandlerStub) BatchCreate(_ context.Context, _ []meetingdomain.Transcript) error {
	return nil
}

func (s *transcriptRepoHandlerStub) ListByMeeting(_ context.Context, _ uint64) ([]meetingdomain.Transcript, error) {
	return nil, nil
}

func (s *transcriptRepoHandlerStub) DeleteByMeeting(_ context.Context, _ uint64) error {
	return nil
}

type summaryRepoHandlerStub struct{}

func (s *summaryRepoHandlerStub) Create(_ context.Context, _ *meetingdomain.Summary) error {
	return nil
}

func (s *summaryRepoHandlerStub) GetByMeeting(_ context.Context, _ uint64) (*meetingdomain.Summary, error) {
	return nil, gorm.ErrRecordNotFound
}

func (s *summaryRepoHandlerStub) Update(_ context.Context, _ *meetingdomain.Summary) error {
	return nil
}

func (s *summaryRepoHandlerStub) DeleteByMeeting(_ context.Context, _ uint64) error {
	return nil
}

type summaryRepoHandlerMemoryStub struct {
	current *meetingdomain.Summary
	created *meetingdomain.Summary
	updated *meetingdomain.Summary
}

func (s *summaryRepoHandlerMemoryStub) Create(_ context.Context, summary *meetingdomain.Summary) error {
	summary.ID = 101
	summary.CreatedAt = time.Now()
	copy := *summary
	s.created = &copy
	s.current = &copy
	return nil
}

func (s *summaryRepoHandlerMemoryStub) GetByMeeting(_ context.Context, _ uint64) (*meetingdomain.Summary, error) {
	if s.current == nil {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *s.current
	return &copy, nil
}

func (s *summaryRepoHandlerMemoryStub) Update(_ context.Context, summary *meetingdomain.Summary) error {
	copy := *summary
	s.updated = &copy
	s.current = &copy
	return nil
}

func (s *summaryRepoHandlerMemoryStub) DeleteByMeeting(_ context.Context, _ uint64) error {
	s.current = nil
	return nil
}

func TestCreateMeetingRejectsNonMeetingWorkflow(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	workflowSvc := appwf.NewService(
		&workflowRepoBindingStub{wf: &wfdomain.Workflow{ID: 12, Name: "实时工作流"}},
		&workflowNodeBindingStub{nodes: []wfdomain.Node{{NodeType: wfdomain.NodeRealtimeASR, Position: 1, Enabled: true}}},
		nil,
		nil,
		nil,
		nil,
	)
	handler := NewMeetingHandler(nil, workflowSvc, "uploads", "", 100, pkgconfig.ProductConfig{Edition: pkgconfig.ProductEditionAdvanced}.Features())

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 8, Role: "user"})
		c.Next()
	})
	router.POST("/meetings", handler.Create)

	body, err := json.Marshal(map[string]any{
		"title":       "周会",
		"audio_url":   "https://example.com/meeting.wav",
		"workflow_id": 12,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/meetings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("不能绑定到会议纪要入口")) {
		t.Fatalf("expected mismatch message, got %s", recorder.Body.String())
	}
}

func TestRegenerateSummaryRejectsNonMeetingWorkflow(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	workflowSvc := appwf.NewService(
		&workflowRepoBindingStub{wf: &wfdomain.Workflow{ID: 13, Name: "批量工作流"}},
		&workflowNodeBindingStub{nodes: []wfdomain.Node{{NodeType: wfdomain.NodeBatchASR, Position: 1, Enabled: true}}},
		nil,
		nil,
		nil,
		nil,
	)
	handler := NewMeetingHandler(nil, workflowSvc, "uploads", "", 100, pkgconfig.ProductConfig{Edition: pkgconfig.ProductEditionAdvanced}.Features())

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 8, Role: "user"})
		c.Next()
	})
	router.POST("/meetings/:id/summary", handler.RegenerateSummary)

	body, err := json.Marshal(map[string]any{
		"workflow_id": 13,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/meetings/1/summary", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("不能绑定到会议纪要入口")) {
		t.Fatalf("expected mismatch message, got %s", recorder.Body.String())
	}
}

func TestDeleteMeetingAllowsCompletedMeeting(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	meetingRepo := &meetingRepoHandlerStub{meeting: &meetingdomain.Meeting{
		ID:        6,
		UserID:    8,
		Status:    meetingdomain.MeetingStatusCompleted,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}
	service := appmeeting.NewService(meetingRepo, &transcriptRepoHandlerStub{}, &summaryRepoHandlerStub{}, nil, nil, nil)
	handler := NewMeetingHandler(service, nil, "uploads", "", 100, pkgconfig.ProductConfig{Edition: pkgconfig.ProductEditionAdvanced}.Features())

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 8, Role: "user"})
		c.Next()
	})
	router.DELETE("/meetings/:id", handler.Delete)

	req := httptest.NewRequest(http.MethodDelete, "/meetings/6", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if meetingRepo.deleted != 6 {
		t.Fatalf("expected meeting 6 deleted, got %d", meetingRepo.deleted)
	}
}

func TestDeleteMeetingRejectsProcessingMeeting(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	meetingRepo := &meetingRepoHandlerStub{meeting: &meetingdomain.Meeting{
		ID:        7,
		UserID:    8,
		Status:    meetingdomain.MeetingStatusProcessing,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}
	service := appmeeting.NewService(meetingRepo, &transcriptRepoHandlerStub{}, &summaryRepoHandlerStub{}, nil, nil, nil)
	handler := NewMeetingHandler(service, nil, "uploads", "", 100, pkgconfig.ProductConfig{Edition: pkgconfig.ProductEditionAdvanced}.Features())

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 8, Role: "user"})
		c.Next()
	})
	router.DELETE("/meetings/:id", handler.Delete)

	req := httptest.NewRequest(http.MethodDelete, "/meetings/7", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if meetingRepo.deleted != 0 {
		t.Fatalf("expected meeting not deleted, got %d", meetingRepo.deleted)
	}
}

func TestUpdateMeetingPersistsUserEdits(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	meetingRepo := &meetingRepoHandlerStub{meeting: &meetingdomain.Meeting{
		ID:        9,
		UserID:    8,
		Title:     "旧标题",
		AudioURL:  "https://example.com/a.wav",
		Status:    meetingdomain.MeetingStatusCompleted,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}
	summaryRepo := &summaryRepoHandlerMemoryStub{current: &meetingdomain.Summary{ID: 19, MeetingID: 9, Content: "旧纪要", ModelVersion: "qwen"}}
	service := appmeeting.NewService(meetingRepo, &transcriptRepoHandlerStub{}, summaryRepo, nil, nil, nil)
	handler := NewMeetingHandler(service, nil, "uploads", "", 100, pkgconfig.ProductConfig{Edition: pkgconfig.ProductEditionAdvanced}.Features())

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 8, Role: "user"})
		c.Next()
	})
	router.PUT("/meetings/:id", handler.Update)

	body, err := json.Marshal(map[string]any{
		"title":           "新标题",
		"summary_content": "# 新纪要",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/meetings/9", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if meetingRepo.meeting == nil || meetingRepo.meeting.Title != "新标题" {
		t.Fatalf("expected meeting title update, got %+v", meetingRepo.meeting)
	}
	if summaryRepo.updated == nil || summaryRepo.updated.Content != "# 新纪要" {
		t.Fatalf("expected summary update, got %+v", summaryRepo.updated)
	}
}
