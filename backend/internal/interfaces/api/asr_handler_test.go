package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	appasr "github.com/lgt/asr/internal/application/asr"
	appwf "github.com/lgt/asr/internal/application/workflow"
	domainasr "github.com/lgt/asr/internal/domain/asr"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
	"github.com/lgt/asr/internal/interfaces/middleware"
)

type taskRepoHandlerStub struct {
	tasks     map[uint64]*domainasr.TranscriptionTask
	deletedID uint64
	updated   *domainasr.TranscriptionTask
}

func (r *taskRepoHandlerStub) Create(_ context.Context, _ *domainasr.TranscriptionTask) error {
	panic("unexpected Create call")
}

func (r *taskRepoHandlerStub) GetByID(_ context.Context, id uint64) (*domainasr.TranscriptionTask, error) {
	task, ok := r.tasks[id]
	if !ok {
		return nil, errors.New("not found")
	}
	copy := *task
	return &copy, nil
}

func (r *taskRepoHandlerStub) Update(_ context.Context, task *domainasr.TranscriptionTask) error {
	r.updated = cloneTaskForHandler(task)
	copy := *task
	r.tasks[task.ID] = &copy
	return nil
}

func (r *taskRepoHandlerStub) Delete(_ context.Context, id uint64) error {
	r.deletedID = id
	delete(r.tasks, id)
	return nil
}

func (r *taskRepoHandlerStub) ListByUser(_ context.Context, _ uint64, _, _ int) ([]*domainasr.TranscriptionTask, int64, error) {
	panic("unexpected ListByUser call")
}

func (r *taskRepoHandlerStub) ListSyncCandidates(_ context.Context, _ int) ([]*domainasr.TranscriptionTask, error) {
	panic("unexpected ListSyncCandidates call")
}

func (r *taskRepoHandlerStub) ListPostProcessRetryCandidates(_ context.Context, _ int) ([]*domainasr.TranscriptionTask, error) {
	panic("unexpected ListPostProcessRetryCandidates call")
}

func (r *taskRepoHandlerStub) SaveLatestRetryResult(_ context.Context, _ *domainasr.RetryPostProcessRecord, _ int) error {
	panic("unexpected SaveLatestRetryResult call")
}

func (r *taskRepoHandlerStub) GetLatestRetryResult(_ context.Context) (*domainasr.RetryPostProcessRecord, error) {
	panic("unexpected GetLatestRetryResult call")
}

func (r *taskRepoHandlerStub) GetRetryHistory(_ context.Context, _ int) ([]*domainasr.RetryPostProcessRecord, error) {
	panic("unexpected GetRetryHistory call")
}

func (r *taskRepoHandlerStub) DeleteRetryHistoryItem(_ context.Context, _ time.Time) error {
	panic("unexpected DeleteRetryHistoryItem call")
}

func (r *taskRepoHandlerStub) ClearRetryHistory(_ context.Context) error {
	panic("unexpected ClearRetryHistory call")
}

func (r *taskRepoHandlerStub) GetSyncHealth(_ context.Context, _, _ int) (*domainasr.SyncHealthOverview, []domainasr.SyncAlert, error) {
	panic("unexpected GetSyncHealth call")
}

type completedTaskProcessorHandlerStub struct{}

func (s *completedTaskProcessorHandlerStub) ProcessCompletedTask(_ context.Context, _ *domainasr.TranscriptionTask) error {
	return nil
}

func (s *completedTaskProcessorHandlerStub) ResumeCompletedTaskFromFailure(_ context.Context, _ *domainasr.TranscriptionTask) error {
	return nil
}

type batchEngineHandlerStub struct {
	submitResult *appasr.BatchSubmitResult
	queryResult  *appasr.BatchTaskStatus
	submitErr    error
	queryErr     error
	lastReq      appasr.BatchSubmitRequest
	queryCalls   int
}

func (s *batchEngineHandlerStub) SubmitBatch(_ context.Context, req appasr.BatchSubmitRequest) (*appasr.BatchSubmitResult, error) {
	s.lastReq = req
	if s.submitErr != nil {
		return nil, s.submitErr
	}
	return s.submitResult, nil
}

func (s *batchEngineHandlerStub) QueryBatchTask(_ context.Context, _ string) (*appasr.BatchTaskStatus, error) {
	s.queryCalls++
	if s.queryErr != nil {
		return nil, s.queryErr
	}
	return s.queryResult, nil
}

func cloneTaskForHandler(task *domainasr.TranscriptionTask) *domainasr.TranscriptionTask {
	if task == nil {
		return nil
	}
	copy := *task
	return &copy
}

type workflowRepoBindingStub struct {
	wf *wfdomain.Workflow
}

func (r *workflowRepoBindingStub) Create(_ context.Context, _ *wfdomain.Workflow) error {
	panic("unexpected Create call")
}

func (r *workflowRepoBindingStub) GetByID(_ context.Context, _ uint64) (*wfdomain.Workflow, error) {
	if r.wf == nil {
		return nil, context.Canceled
	}
	copy := *r.wf
	return &copy, nil
}

func (r *workflowRepoBindingStub) Update(_ context.Context, _ *wfdomain.Workflow) error {
	return nil
}

func (r *workflowRepoBindingStub) Delete(_ context.Context, _ uint64) error {
	panic("unexpected Delete call")
}

func (r *workflowRepoBindingStub) List(_ context.Context, _ *wfdomain.OwnerType, _ *uint64, _ bool, _, _ int) ([]*wfdomain.Workflow, int64, error) {
	panic("unexpected List call")
}

func (r *workflowRepoBindingStub) ListFiltered(_ context.Context, _ *wfdomain.OwnerType, _ *uint64, _ bool, _ wfdomain.WorkflowListFilter, _, _ int) ([]*wfdomain.Workflow, int64, error) {
	panic("unexpected ListFiltered call")
}

type workflowNodeBindingStub struct {
	nodes []wfdomain.Node
}

func (r *workflowNodeBindingStub) ListByWorkflow(_ context.Context, _ uint64) ([]wfdomain.Node, error) {
	return append([]wfdomain.Node(nil), r.nodes...), nil
}

func (r *workflowNodeBindingStub) BatchSave(_ context.Context, _ uint64, _ []wfdomain.Node) error {
	panic("unexpected BatchSave call")
}

func (r *workflowNodeBindingStub) DeleteByWorkflow(_ context.Context, _ uint64) error {
	panic("unexpected DeleteByWorkflow call")
}

func TestCreateTaskRejectsWorkflowTypeMismatch(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	workflowSvc := appwf.NewService(
		&workflowRepoBindingStub{wf: &wfdomain.Workflow{ID: 9, Name: "批量工作流"}},
		&workflowNodeBindingStub{nodes: []wfdomain.Node{{NodeType: wfdomain.NodeBatchASR, Position: 1, Enabled: true}}},
		nil,
		nil,
		nil,
	)
	handler := NewASRHandler(nil, workflowSvc, "uploads", "", 100)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 7, Role: "user"})
		c.Next()
	})
	router.POST("/tasks", handler.CreateTask)

	body, err := json.Marshal(map[string]any{
		"type":        domainasr.TaskTypeRealtime,
		"result_text": "hello",
		"workflow_id": 9,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("不能绑定到实时语音识别入口")) {
		t.Fatalf("expected mismatch message, got %s", recorder.Body.String())
	}
}

func TestDeleteTaskAllowsFailedTask(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := &taskRepoHandlerStub{tasks: map[uint64]*domainasr.TranscriptionTask{
		9: {
			ID:                9,
			UserID:            7,
			Type:              domainasr.TaskTypeBatch,
			Status:            domainasr.TaskStatusFailed,
			PostProcessStatus: domainasr.PostProcessPending,
		},
	}}
	handler := NewASRHandler(appasr.NewService(repo, nil, nil, 5, nil), nil, "uploads", "", 100)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 7, Role: "user"})
		c.Next()
	})
	router.DELETE("/tasks/:id", handler.DeleteTask)

	req := httptest.NewRequest(http.MethodDelete, "/tasks/9", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.deletedID != 9 {
		t.Fatalf("expected task 9 to be deleted, got %d", repo.deletedID)
	}
}

func TestDeleteTaskRejectsActiveTask(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := &taskRepoHandlerStub{tasks: map[uint64]*domainasr.TranscriptionTask{
		9: {
			ID:                9,
			UserID:            7,
			Type:              domainasr.TaskTypeBatch,
			Status:            domainasr.TaskStatusProcessing,
			PostProcessStatus: domainasr.PostProcessPending,
		},
	}}
	handler := NewASRHandler(appasr.NewService(repo, nil, nil, 5, nil), nil, "uploads", "", 100)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 7, Role: "user"})
		c.Next()
	})
	router.DELETE("/tasks/:id", handler.DeleteTask)

	req := httptest.NewRequest(http.MethodDelete, "/tasks/9", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.deletedID != 0 {
		t.Fatalf("expected task not to be deleted, got %d", repo.deletedID)
	}
}

func TestResumeTaskPostProcessAllowsFailedBatchTask(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	workflowID := uint64(8)
	repo := &taskRepoHandlerStub{tasks: map[uint64]*domainasr.TranscriptionTask{
		9: {
			ID:                9,
			UserID:            7,
			Type:              domainasr.TaskTypeBatch,
			Status:            domainasr.TaskStatusCompleted,
			WorkflowID:        &workflowID,
			PostProcessStatus: domainasr.PostProcessFailed,
			PostProcessError:  "node failed",
		},
	}}
	handler := NewASRHandler(appasr.NewService(repo, nil, &completedTaskProcessorHandlerStub{}, 5, nil), nil, "uploads", "", 100)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 7, Role: "user"})
		c.Next()
	})
	router.POST("/tasks/:id/resume-post-process", handler.ResumeTaskPostProcess)

	req := httptest.NewRequest(http.MethodPost, "/tasks/9/resume-post-process", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.updated == nil {
		t.Fatal("expected task update after resuming post process")
	}
	if repo.updated.PostProcessStatus != domainasr.PostProcessCompleted {
		t.Fatalf("expected completed status, got %q", repo.updated.PostProcessStatus)
	}
}

func TestTranscribeRealtimeSegmentReturnsSnippetText(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	batchEngine := &batchEngineHandlerStub{
		submitResult: &appasr.BatchSubmitResult{Status: "completed", ResultText: "短句识别结果", Duration: 1.8},
	}
	handler := NewASRHandler(appasr.NewService(nil, batchEngine, nil, 5, nil), nil, "uploads", "", 100)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 7, Role: "user"})
		c.Next()
	})
	router.POST("/realtime-segments", handler.TranscribeRealtimeSegment)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "snippet.wav")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("RIFF....WAVEfmt ")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/realtime-segments", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if batchEngine.lastReq.LocalFilePath == "" {
		t.Fatal("expected local file path to be passed to batch engine")
	}
	if filepath.Ext(batchEngine.lastReq.LocalFilePath) != ".wav" {
		t.Fatalf("expected wav temp file, got %s", batchEngine.lastReq.LocalFilePath)
	}
	if _, err := os.Stat(batchEngine.lastReq.LocalFilePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected temp file to be removed, stat err=%v", err)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("短句识别结果")) {
		t.Fatalf("expected snippet text in response, got %s", recorder.Body.String())
	}
	if batchEngine.queryCalls != 0 {
		t.Fatalf("expected no query polling for immediate response, got %d", batchEngine.queryCalls)
	}
}

func TestTranscribeRealtimeSegmentPollsTaskResult(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	batchEngine := &batchEngineHandlerStub{
		submitResult: &appasr.BatchSubmitResult{TaskID: "task-1", Status: "processing"},
		queryResult:  &appasr.BatchTaskStatus{Status: "completed", ResultText: "轮询后的识别结果", Duration: 2.2},
	}
	handler := NewASRHandler(appasr.NewService(nil, batchEngine, nil, 5, nil), nil, "uploads", "", 100)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 7, Role: "user"})
		c.Next()
	})
	router.POST("/realtime-segments", handler.TranscribeRealtimeSegment)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "snippet.wav")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("RIFF....WAVEfmt ")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/realtime-segments", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if batchEngine.queryCalls != 1 {
		t.Fatalf("expected one query polling call, got %d", batchEngine.queryCalls)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("轮询后的识别结果")) {
		t.Fatalf("expected polled snippet text in response, got %s", recorder.Body.String())
	}
}
