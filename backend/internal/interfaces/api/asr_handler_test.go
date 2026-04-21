package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type responseEnvelope[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type taskRepoHandlerStub struct {
	tasks     map[uint64]*domainasr.TranscriptionTask
	deletedID uint64
	updated   *domainasr.TranscriptionTask
}

func (r *taskRepoHandlerStub) Create(_ context.Context, task *domainasr.TranscriptionTask) error {
	if r.tasks == nil {
		r.tasks = map[uint64]*domainasr.TranscriptionTask{}
	}
	if task.ID == 0 {
		task.ID = uint64(len(r.tasks) + 1)
	}
	copy := *task
	r.tasks[task.ID] = &copy
	return nil
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

func (r *taskRepoHandlerStub) ListByUser(_ context.Context, userID uint64, taskType *domainasr.TaskType, offset, limit int) ([]*domainasr.TranscriptionTask, int64, error) {
	items := make([]*domainasr.TranscriptionTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task.UserID != userID {
			continue
		}
		if taskType != nil && task.Type != *taskType {
			continue
		}
		items = append(items, cloneTaskForHandler(task))
	}
	if offset >= len(items) {
		return []*domainasr.TranscriptionTask{}, int64(len(items)), nil
	}
	if limit <= 0 {
		limit = len(items)
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], int64(len(items)), nil
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
	submitResult        *appasr.BatchSubmitResult
	queryResult         *appasr.BatchTaskStatus
	submitErr           error
	queryErr            error
	streamStartErr      error
	streamSessionID     string
	streamChunkResult   *appasr.StreamChunkResponse
	streamChunkErr      error
	streamFinishResult  *appasr.StreamChunkResponse
	streamFinishErr     error
	lastReq             appasr.BatchSubmitRequest
	lastStreamSessionID string
	lastStreamPCMData   []byte
	finishedSessionID   string
	queryCalls          int
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

func (s *batchEngineHandlerStub) StartStreamSession(_ context.Context) (string, error) {
	if s.streamStartErr != nil {
		return "", s.streamStartErr
	}
	return s.streamSessionID, nil
}

func (s *batchEngineHandlerStub) PushStreamChunk(_ context.Context, sessionID string, pcmData []byte) (*appasr.StreamChunkResponse, error) {
	s.lastStreamSessionID = sessionID
	s.lastStreamPCMData = append([]byte(nil), pcmData...)
	if s.streamChunkErr != nil {
		return nil, s.streamChunkErr
	}
	return s.streamChunkResult, nil
}

func (s *batchEngineHandlerStub) FinishStreamSession(_ context.Context, sessionID string) (*appasr.StreamChunkResponse, error) {
	s.finishedSessionID = sessionID
	if s.streamFinishErr != nil {
		return nil, s.streamFinishErr
	}
	return s.streamFinishResult, nil
}

func cloneTaskForHandler(task *domainasr.TranscriptionTask) *domainasr.TranscriptionTask {
	if task == nil {
		return nil
	}
	copy := *task
	return &copy
}

func startManagedStreamSession(t *testing.T, router *gin.Engine) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/stream-sessions", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200 when starting stream session, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var envelope responseEnvelope[appasr.StreamSessionResponse]
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stream session response: %v", err)
	}
	if envelope.Data.SessionID == "" {
		t.Fatalf("expected non-empty managed session id, got %+v", envelope)
	}
	return envelope.Data.SessionID
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

func TestClearTasksSupportsTaskTypeFilter(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := &taskRepoHandlerStub{tasks: map[uint64]*domainasr.TranscriptionTask{
		1: {
			ID:                1,
			UserID:            7,
			Type:              domainasr.TaskTypeRealtime,
			Status:            domainasr.TaskStatusCompleted,
			PostProcessStatus: domainasr.PostProcessCompleted,
		},
		2: {
			ID:                2,
			UserID:            7,
			Type:              domainasr.TaskTypeBatch,
			Status:            domainasr.TaskStatusCompleted,
			PostProcessStatus: domainasr.PostProcessCompleted,
		},
	}}
	handler := NewASRHandler(appasr.NewService(repo, nil, nil, 5, nil), nil, "uploads", "", 100)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 7, Role: "user"})
		c.Next()
	})
	router.DELETE("/tasks", handler.ClearTasks)

	req := httptest.NewRequest(http.MethodDelete, "/tasks?type=realtime", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if _, ok := repo.tasks[1]; ok {
		t.Fatal("expected realtime task removed")
	}
	if _, ok := repo.tasks[2]; !ok {
		t.Fatal("expected batch task to remain")
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("deleted_count")) {
		t.Fatalf("expected clear response body, got %s", recorder.Body.String())
	}
}

func TestListTasksRejectsInvalidTaskType(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	handler := NewASRHandler(appasr.NewService(&taskRepoHandlerStub{}, nil, nil, 5, nil), nil, "uploads", "", 100)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 7, Role: "user"})
		c.Next()
	})
	router.GET("/tasks", handler.ListTasks)

	req := httptest.NewRequest(http.MethodGet, "/tasks?type=unknown", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", recorder.Code, recorder.Body.String())
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

func TestUploadRealtimeTaskFileCreatesCompletedTaskWithAudio(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := &taskRepoHandlerStub{tasks: map[uint64]*domainasr.TranscriptionTask{}}
	handler := NewASRHandler(appasr.NewService(repo, nil, &completedTaskProcessorHandlerStub{}, 5, nil), nil, t.TempDir(), "http://example.com", 100)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 7, Role: "user"})
		c.Next()
	})
	router.POST("/realtime-tasks/upload", handler.UploadRealtimeTaskFile)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "session.wav")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("RIFF....WAVEfmt ")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.WriteField("result_text", "整段实时文本"); err != nil {
		t.Fatalf("write result_text: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/realtime-tasks/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if len(repo.tasks) != 1 {
		t.Fatalf("expected one realtime task created, got %d", len(repo.tasks))
	}
	var created *domainasr.TranscriptionTask
	for _, task := range repo.tasks {
		created = task
	}
	if created == nil {
		t.Fatal("expected created task")
	}
	if created.Type != domainasr.TaskTypeRealtime {
		t.Fatalf("expected realtime task type, got %q", created.Type)
	}
	if created.ResultText != "整段实时文本" {
		t.Fatalf("unexpected result text: %s", created.ResultText)
	}
	if created.LocalFilePath == "" {
		t.Fatalf("expected local file path on created task, got %+v", created)
	}
	if created.AudioURL == "" {
		t.Fatalf("expected audio url on created task, got %+v", created)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("session.wav")) {
		t.Fatalf("expected filename in response, got %s", recorder.Body.String())
	}
	if _, err := os.Stat(created.LocalFilePath); err != nil {
		t.Fatalf("expected uploaded session audio kept on disk, stat err=%v", err)
	}
	if err := os.Remove(created.LocalFilePath); err != nil {
		t.Fatalf("cleanup created audio file: %v", err)
	}
}

func TestStartStreamSessionReturnsSessionID(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	batchEngine := &batchEngineHandlerStub{streamSessionID: "upstream-stream-1"}
	handler := NewASRHandler(appasr.NewService(nil, batchEngine, nil, 5, nil), nil, "uploads", "", 100)

	router := gin.New()
	router.POST("/stream-sessions", handler.StartStreamSession)

	req := httptest.NewRequest(http.MethodPost, "/stream-sessions", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope responseEnvelope[appasr.StreamSessionResponse]
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if envelope.Data.SessionID == "" {
		t.Fatalf("expected non-empty managed session id, got %s", recorder.Body.String())
	}
	if envelope.Data.SessionID == "upstream-stream-1" {
		t.Fatalf("expected managed session id different from upstream id, got %s", recorder.Body.String())
	}
}

func TestPushStreamChunkForwardsRawPCMData(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	batchEngine := &batchEngineHandlerStub{streamSessionID: "upstream-stream-1", streamChunkResult: &appasr.StreamChunkResponse{SessionID: "upstream-stream-1", Text: "增量文本", Language: "zh"}}
	handler := NewASRHandler(appasr.NewService(nil, batchEngine, nil, 5, nil), nil, "uploads", "", 100)

	router := gin.New()
	router.POST("/stream-sessions", handler.StartStreamSession)
	router.POST("/stream-sessions/:id/chunks", handler.PushStreamChunk)
	router.POST("/stream-sessions/:id/commit", handler.CommitStreamSession)
	managedSessionID := startManagedStreamSession(t, router)

	payload := []byte{1, 2, 3, 4}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/stream-sessions/%s/chunks", managedSessionID), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/octet-stream")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if batchEngine.lastStreamSessionID != "upstream-stream-1" {
		t.Fatalf("expected stream session id forwarded, got %s", batchEngine.lastStreamSessionID)
	}
	if !bytes.Equal(batchEngine.lastStreamPCMData, payload) {
		t.Fatalf("expected raw pcm data forwarded, got %v", batchEngine.lastStreamPCMData)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("增量文本")) {
		t.Fatalf("expected chunk text in response, got %s", recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("text_delta")) {
		t.Fatalf("expected chunk delta in response, got %s", recorder.Body.String())
	}
}

func TestCommitStreamSessionReturnsCommittedDelta(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	batchEngine := &batchEngineHandlerStub{streamSessionID: "upstream-stream-1", streamChunkResult: &appasr.StreamChunkResponse{SessionID: "upstream-stream-1", Text: "你好世界", Language: "zh"}}
	handler := NewASRHandler(appasr.NewService(nil, batchEngine, nil, 5, nil), nil, "uploads", "", 100)

	router := gin.New()
	router.POST("/stream-sessions", handler.StartStreamSession)
	router.POST("/stream-sessions/:id/chunks", handler.PushStreamChunk)
	router.POST("/stream-sessions/:id/commit", handler.CommitStreamSession)
	managedSessionID := startManagedStreamSession(t, router)

	chunkReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/stream-sessions/%s/chunks", managedSessionID), bytes.NewReader([]byte{1, 2, 3}))
	chunkReq.Header.Set("Content-Type", "application/octet-stream")
	chunkRecorder := httptest.NewRecorder()
	router.ServeHTTP(chunkRecorder, chunkReq)
	if chunkRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200 for chunk, got %d, body=%s", chunkRecorder.Code, chunkRecorder.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/stream-sessions/%s/commit", managedSessionID), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("你好世界")) {
		t.Fatalf("expected committed text in response, got %s", recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("text_delta")) {
		t.Fatalf("expected committed delta in response, got %s", recorder.Body.String())
	}
}

func TestFinishStreamSessionReturnsFinalText(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	batchEngine := &batchEngineHandlerStub{streamSessionID: "upstream-stream-1", streamFinishResult: &appasr.StreamChunkResponse{SessionID: "upstream-stream-1", Text: "最终文本", Language: "zh"}}
	handler := NewASRHandler(appasr.NewService(nil, batchEngine, nil, 5, nil), nil, "uploads", "", 100)

	router := gin.New()
	router.POST("/stream-sessions", handler.StartStreamSession)
	router.POST("/stream-sessions/:id/finish", handler.FinishStreamSession)
	managedSessionID := startManagedStreamSession(t, router)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/stream-sessions/%s/finish", managedSessionID), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if batchEngine.finishedSessionID != "upstream-stream-1" {
		t.Fatalf("expected finish session id forwarded, got %s", batchEngine.finishedSessionID)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("最终文本")) {
		t.Fatalf("expected final text in response, got %s", recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("is_final")) {
		t.Fatalf("expected final flag in response, got %s", recorder.Body.String())
	}
}

func TestPushStreamChunkRejectsUnknownSession(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	handler := NewASRHandler(appasr.NewService(nil, &batchEngineHandlerStub{}, nil, 5, nil), nil, "uploads", "", 100)
	router := gin.New()
	router.POST("/stream-sessions/:id/chunks", handler.PushStreamChunk)

	req := httptest.NewRequest(http.MethodPost, "/stream-sessions/missing/chunks", bytes.NewReader([]byte{1, 2, 3}))
	req.Header.Set("Content-Type", "application/octet-stream")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}
