package asr

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/lgt/asr/internal/domain/asr"
)

type taskRepoServiceStub struct {
	tasks     map[uint64]*domain.TranscriptionTask
	deletedID uint64
}

type completedTaskProcessorStub struct {
	resumeErr     error
	resumedTask   *domain.TranscriptionTask
	processedTask *domain.TranscriptionTask
}

type streamingBatchEngineServiceStub struct {
	startSessionID string
	startErr       error
	chunkResult    *StreamChunkResponse
	chunkErr       error
	finishResult   *StreamChunkResponse
	finishErr      error
	lastSessionID  string
	lastPCMData    []byte
	finishSession  string
}

func (s *completedTaskProcessorStub) ProcessCompletedTask(_ context.Context, task *domain.TranscriptionTask) error {
	s.processedTask = cloneTask(task)
	return nil
}

func (s *completedTaskProcessorStub) ResumeCompletedTaskFromFailure(_ context.Context, task *domain.TranscriptionTask) error {
	s.resumedTask = cloneTask(task)
	return s.resumeErr
}

func (s *streamingBatchEngineServiceStub) SubmitBatch(_ context.Context, _ BatchSubmitRequest) (*BatchSubmitResult, error) {
	panic("unexpected SubmitBatch call")
}

func (s *streamingBatchEngineServiceStub) QueryBatchTask(_ context.Context, _ string) (*BatchTaskStatus, error) {
	panic("unexpected QueryBatchTask call")
}

func (s *streamingBatchEngineServiceStub) StartStreamSession(_ context.Context) (string, error) {
	if s.startErr != nil {
		return "", s.startErr
	}
	return s.startSessionID, nil
}

func (s *streamingBatchEngineServiceStub) PushStreamChunk(_ context.Context, sessionID string, pcmData []byte) (*StreamChunkResponse, error) {
	s.lastSessionID = sessionID
	s.lastPCMData = append([]byte(nil), pcmData...)
	if s.chunkErr != nil {
		return nil, s.chunkErr
	}
	return s.chunkResult, nil
}

func (s *streamingBatchEngineServiceStub) FinishStreamSession(_ context.Context, sessionID string) (*StreamChunkResponse, error) {
	s.finishSession = sessionID
	if s.finishErr != nil {
		return nil, s.finishErr
	}
	return s.finishResult, nil
}

func (r *taskRepoServiceStub) Create(_ context.Context, task *domain.TranscriptionTask) error {
	if r.tasks == nil {
		r.tasks = map[uint64]*domain.TranscriptionTask{}
	}
	if task.ID == 0 {
		task.ID = uint64(len(r.tasks) + 1)
	}
	r.tasks[task.ID] = cloneTask(task)
	return nil
}

func (r *taskRepoServiceStub) GetByID(_ context.Context, id uint64) (*domain.TranscriptionTask, error) {
	task, ok := r.tasks[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return cloneTask(task), nil
}

func (r *taskRepoServiceStub) Update(_ context.Context, task *domain.TranscriptionTask) error {
	r.tasks[task.ID] = cloneTask(task)
	return nil
}

func (r *taskRepoServiceStub) Delete(_ context.Context, id uint64) error {
	r.deletedID = id
	delete(r.tasks, id)
	return nil
}

func (r *taskRepoServiceStub) ListByUser(_ context.Context, userID uint64, taskType *domain.TaskType, offset, limit int) ([]*domain.TranscriptionTask, int64, error) {
	items := make([]*domain.TranscriptionTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task.UserID != userID {
			continue
		}
		if taskType != nil && task.Type != *taskType {
			continue
		}
		items = append(items, cloneTask(task))
	}
	if offset >= len(items) {
		return []*domain.TranscriptionTask{}, int64(len(items)), nil
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

func (r *taskRepoServiceStub) ListSyncCandidates(_ context.Context, _ int) ([]*domain.TranscriptionTask, error) {
	return nil, nil
}

func (r *taskRepoServiceStub) ListPostProcessRetryCandidates(_ context.Context, _ int) ([]*domain.TranscriptionTask, error) {
	return nil, nil
}

func (r *taskRepoServiceStub) SaveLatestRetryResult(_ context.Context, _ *domain.RetryPostProcessRecord, _ int) error {
	return nil
}

func (r *taskRepoServiceStub) GetLatestRetryResult(_ context.Context) (*domain.RetryPostProcessRecord, error) {
	return nil, nil
}

func (r *taskRepoServiceStub) GetRetryHistory(_ context.Context, _ int) ([]*domain.RetryPostProcessRecord, error) {
	return nil, nil
}

func (r *taskRepoServiceStub) DeleteRetryHistoryItem(_ context.Context, _ time.Time) error {
	return nil
}

func (r *taskRepoServiceStub) ClearRetryHistory(_ context.Context) error {
	return nil
}

func (r *taskRepoServiceStub) GetSyncHealth(_ context.Context, _, _ int) (*domain.SyncHealthOverview, []domain.SyncAlert, error) {
	return nil, nil, nil
}

func cloneTask(task *domain.TranscriptionTask) *domain.TranscriptionTask {
	if task == nil {
		return nil
	}
	copy := *task
	return &copy
}

func TestDeleteTaskAllowsFailedTaskAndRemovesLocalFile(t *testing.T) {
	localDir := t.TempDir()
	localFile := filepath.Join(localDir, "failed.wav")
	if err := os.WriteFile(localFile, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}

	repo := &taskRepoServiceStub{tasks: map[uint64]*domain.TranscriptionTask{
		3: {
			ID:                3,
			UserID:            7,
			Type:              domain.TaskTypeBatch,
			Status:            domain.TaskStatusFailed,
			PostProcessStatus: domain.PostProcessPending,
			LocalFilePath:     localFile,
		},
	}}
	service := NewService(repo, nil, nil, 5, nil)

	if err := service.DeleteTask(context.Background(), 7, 3); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	if repo.deletedID != 3 {
		t.Fatalf("expected task 3 to be deleted, got %d", repo.deletedID)
	}
	if _, err := os.Stat(localFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected local file removed, stat err=%v", err)
	}
}

func TestDeleteTaskRejectsProcessingTask(t *testing.T) {
	repo := &taskRepoServiceStub{tasks: map[uint64]*domain.TranscriptionTask{
		4: {
			ID:                4,
			UserID:            7,
			Type:              domain.TaskTypeBatch,
			Status:            domain.TaskStatusProcessing,
			PostProcessStatus: domain.PostProcessPending,
		},
	}}
	service := NewService(repo, nil, nil, 5, nil)

	err := service.DeleteTask(context.Background(), 7, 4)
	if !errors.Is(err, ErrTaskDeleteNotAllowed) {
		t.Fatalf("expected ErrTaskDeleteNotAllowed, got %v", err)
	}
	if repo.deletedID != 0 {
		t.Fatalf("expected task not deleted, got %d", repo.deletedID)
	}
}

func TestDeleteTaskRejectsForeignTask(t *testing.T) {
	repo := &taskRepoServiceStub{tasks: map[uint64]*domain.TranscriptionTask{
		5: {
			ID:                5,
			UserID:            8,
			Type:              domain.TaskTypeBatch,
			Status:            domain.TaskStatusFailed,
			PostProcessStatus: domain.PostProcessPending,
		},
	}}
	service := NewService(repo, nil, nil, 5, nil)

	err := service.DeleteTask(context.Background(), 7, 5)
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
	if repo.deletedID != 0 {
		t.Fatalf("expected task not deleted, got %d", repo.deletedID)
	}
}

func TestClearTasksDeletesOnlyOwnedDeletableTasks(t *testing.T) {
	localDir := t.TempDir()
	firstFile := filepath.Join(localDir, "first.wav")
	if err := os.WriteFile(firstFile, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}

	realtimeType := domain.TaskTypeRealtime
	repo := &taskRepoServiceStub{tasks: map[uint64]*domain.TranscriptionTask{
		1: {
			ID:                1,
			UserID:            7,
			Type:              domain.TaskTypeRealtime,
			Status:            domain.TaskStatusCompleted,
			PostProcessStatus: domain.PostProcessCompleted,
			LocalFilePath:     firstFile,
		},
		2: {
			ID:                2,
			UserID:            7,
			Type:              domain.TaskTypeRealtime,
			Status:            domain.TaskStatusProcessing,
			PostProcessStatus: domain.PostProcessPending,
		},
		3: {
			ID:                3,
			UserID:            7,
			Type:              domain.TaskTypeBatch,
			Status:            domain.TaskStatusCompleted,
			PostProcessStatus: domain.PostProcessCompleted,
		},
		4: {
			ID:                4,
			UserID:            8,
			Type:              domain.TaskTypeRealtime,
			Status:            domain.TaskStatusCompleted,
			PostProcessStatus: domain.PostProcessCompleted,
		},
	}}
	service := NewService(repo, nil, nil, 5, nil)

	result, err := service.ClearTasks(context.Background(), 7, &realtimeType)
	if err != nil {
		t.Fatalf("clear tasks: %v", err)
	}
	if result.DeletedCount != 1 {
		t.Fatalf("expected one task deleted, got %+v", result)
	}
	if result.SkippedCount != 1 {
		t.Fatalf("expected one task skipped, got %+v", result)
	}
	if _, ok := repo.tasks[1]; ok {
		t.Fatal("expected realtime completed task to be deleted")
	}
	if _, ok := repo.tasks[2]; !ok {
		t.Fatal("expected processing realtime task to remain")
	}
	if _, ok := repo.tasks[3]; !ok {
		t.Fatal("expected other task type to remain")
	}
	if _, err := os.Stat(firstFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected local file removed, stat err=%v", err)
	}
}

func TestResumeTaskPostProcessFromFailure(t *testing.T) {
	workflowID := uint64(12)
	repo := &taskRepoServiceStub{tasks: map[uint64]*domain.TranscriptionTask{
		6: {
			ID:                6,
			UserID:            7,
			Type:              domain.TaskTypeBatch,
			Status:            domain.TaskStatusCompleted,
			WorkflowID:        &workflowID,
			PostProcessStatus: domain.PostProcessFailed,
			PostProcessError:  "node failed",
		},
	}}
	processor := &completedTaskProcessorStub{}
	service := NewService(repo, nil, processor, 5, nil)

	result, err := service.ResumeTaskPostProcessFromFailure(context.Background(), 7, 6)
	if err != nil {
		t.Fatalf("ResumeTaskPostProcessFromFailure returned error: %v", err)
	}
	if processor.resumedTask == nil || processor.resumedTask.ID != 6 {
		t.Fatalf("expected processor to resume task 6, got %+v", processor.resumedTask)
	}
	if result.PostProcessStatus != domain.PostProcessCompleted {
		t.Fatalf("expected post process status completed, got %q", result.PostProcessStatus)
	}
	stored, _ := repo.GetByID(context.Background(), 6)
	if stored.PostProcessStatus != domain.PostProcessCompleted {
		t.Fatalf("expected stored task status completed, got %q", stored.PostProcessStatus)
	}
	if stored.PostProcessError != "" {
		t.Fatalf("expected stored error cleared, got %q", stored.PostProcessError)
	}
}

func TestResumeTaskPostProcessFromFailureRejectsInvalidTask(t *testing.T) {
	repo := &taskRepoServiceStub{tasks: map[uint64]*domain.TranscriptionTask{
		6: {
			ID:                6,
			UserID:            7,
			Type:              domain.TaskTypeBatch,
			Status:            domain.TaskStatusProcessing,
			PostProcessStatus: domain.PostProcessFailed,
		},
	}}
	service := NewService(repo, nil, &completedTaskProcessorStub{}, 5, nil)

	_, err := service.ResumeTaskPostProcessFromFailure(context.Background(), 7, 6)
	if !errors.Is(err, ErrTaskResumeNotAllowed) {
		t.Fatalf("expected ErrTaskResumeNotAllowed, got %v", err)
	}
}

func TestStartStreamSessionUsesStreamingEngine(t *testing.T) {
	engine := &streamingBatchEngineServiceStub{startSessionID: "upstream-stream-1"}
	service := NewService(nil, engine, nil, 5, nil)

	result, err := service.StartStreamSession(context.Background())
	if err != nil {
		t.Fatalf("StartStreamSession returned error: %v", err)
	}
	if result == nil || result.SessionID == "" {
		t.Fatalf("expected non-empty managed session id, got %+v", result)
	}
	if result.SessionID == "upstream-stream-1" {
		t.Fatalf("expected managed session id different from upstream id, got %+v", result)
	}
}

func TestPushStreamChunkForwardsPCMData(t *testing.T) {
	engine := &streamingBatchEngineServiceStub{startSessionID: "upstream-stream-1", chunkResult: &StreamChunkResponse{SessionID: "upstream-stream-1", Text: "增量文本", Language: "zh"}}
	service := NewService(nil, engine, nil, 5, nil)
	started, err := service.StartStreamSession(context.Background())
	if err != nil {
		t.Fatalf("StartStreamSession returned error: %v", err)
	}
	payload := []byte{1, 2, 3, 4}

	result, err := service.PushStreamChunk(context.Background(), &PushStreamChunkRequest{SessionID: started.SessionID, PCMData: payload})
	if err != nil {
		t.Fatalf("PushStreamChunk returned error: %v", err)
	}
	if result == nil || result.Text != "增量文本" {
		t.Fatalf("expected chunk text, got %+v", result)
	}
	if result.TextDelta != "增量文本" {
		t.Fatalf("expected text delta 增量文本, got %+v", result)
	}
	if engine.lastSessionID != "upstream-stream-1" {
		t.Fatalf("expected session id stream-1, got %s", engine.lastSessionID)
	}
	if !bytes.Equal(engine.lastPCMData, payload) {
		t.Fatalf("expected pcm payload forwarded, got %v", engine.lastPCMData)
	}
}

func TestCommitStreamSegmentReturnsCommittedDelta(t *testing.T) {
	engine := &streamingBatchEngineServiceStub{startSessionID: "upstream-stream-1", chunkResult: &StreamChunkResponse{SessionID: "upstream-stream-1", Text: "你好世界", Language: "zh"}}
	service := NewService(nil, engine, nil, 5, nil)
	started, err := service.StartStreamSession(context.Background())
	if err != nil {
		t.Fatalf("StartStreamSession returned error: %v", err)
	}

	if _, err := service.PushStreamChunk(context.Background(), &PushStreamChunkRequest{SessionID: started.SessionID, PCMData: []byte{1, 2, 3, 4}}); err != nil {
		t.Fatalf("PushStreamChunk returned error: %v", err)
	}

	result, err := service.CommitStreamSegment(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("CommitStreamSegment returned error: %v", err)
	}
	if result == nil || result.Text != "你好世界" {
		t.Fatalf("expected committed cumulative text, got %+v", result)
	}
	if result.TextDelta != "你好世界" {
		t.Fatalf("expected committed delta 你好世界, got %+v", result)
	}

	result, err = service.CommitStreamSegment(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("second CommitStreamSegment returned error: %v", err)
	}
	if result.TextDelta != "" {
		t.Fatalf("expected empty delta on repeated commit, got %+v", result)
	}
}

func TestFinishStreamSessionUsesStreamingEngine(t *testing.T) {
	engine := &streamingBatchEngineServiceStub{startSessionID: "upstream-stream-1", finishResult: &StreamChunkResponse{SessionID: "upstream-stream-1", Text: "最终文本", Language: "zh"}}
	service := NewService(nil, engine, nil, 5, nil)
	started, err := service.StartStreamSession(context.Background())
	if err != nil {
		t.Fatalf("StartStreamSession returned error: %v", err)
	}

	result, err := service.FinishStreamSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("FinishStreamSession returned error: %v", err)
	}
	if result == nil || result.Text != "最终文本" {
		t.Fatalf("expected final text, got %+v", result)
	}
	if result.TextDelta != "最终文本" {
		t.Fatalf("expected final committed delta, got %+v", result)
	}
	if !result.IsFinal {
		t.Fatalf("expected final response flag, got %+v", result)
	}
	if engine.finishSession != "upstream-stream-1" {
		t.Fatalf("expected finished session stream-1, got %s", engine.finishSession)
	}
}

func TestGetStreamSessionStateTracksTranscriptAndCommit(t *testing.T) {
	engine := &streamingBatchEngineServiceStub{
		startSessionID: "upstream-stream-1",
		chunkResult:    &StreamChunkResponse{SessionID: "upstream-stream-1", Text: "你好", Language: "zh"},
		finishResult:   &StreamChunkResponse{SessionID: "upstream-stream-1", Text: "你好世界", Language: "zh"},
	}
	service := NewService(nil, engine, nil, 5, nil)
	started, err := service.StartStreamSession(context.Background())
	if err != nil {
		t.Fatalf("StartStreamSession returned error: %v", err)
	}

	if _, err := service.PushStreamChunk(context.Background(), &PushStreamChunkRequest{SessionID: started.SessionID, PCMData: []byte{1, 2, 3, 4}}); err != nil {
		t.Fatalf("PushStreamChunk returned error: %v", err)
	}
	state, err := service.GetStreamSessionState(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetStreamSessionState returned error after push: %v", err)
	}
	if state.Text != "你好" || state.CommittedText != "" || state.IsFinal {
		t.Fatalf("unexpected state after push: %+v", state)
	}

	if _, err := service.CommitStreamSegment(context.Background(), started.SessionID); err != nil {
		t.Fatalf("CommitStreamSegment returned error: %v", err)
	}
	state, err = service.GetStreamSessionState(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetStreamSessionState returned error after commit: %v", err)
	}
	if state.CommittedText != "你好" || state.IsFinal {
		t.Fatalf("unexpected state after commit: %+v", state)
	}

	if _, err := service.FinishStreamSession(context.Background(), started.SessionID); err != nil {
		t.Fatalf("FinishStreamSession returned error: %v", err)
	}
	state, err = service.GetStreamSessionState(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetStreamSessionState returned error after finish: %v", err)
	}
	if state.Text != "你好世界" || state.CommittedText != "你好世界" || !state.IsFinal {
		t.Fatalf("unexpected final state: %+v", state)
	}
}

func TestCreateRealtimeTaskConsumesFinalizedStreamSessionAudio(t *testing.T) {
	repo := &taskRepoServiceStub{}
	engine := &streamingBatchEngineServiceStub{
		startSessionID: "upstream-stream-1",
		chunkResult:    &StreamChunkResponse{SessionID: "upstream-stream-1", Text: "实时文本", Language: "zh"},
		finishResult:   &StreamChunkResponse{SessionID: "upstream-stream-1", Text: "实时文本", Language: "zh"},
	}
	service := NewService(repo, engine, nil, 5, nil)
	started, err := service.StartStreamSession(context.Background())
	if err != nil {
		t.Fatalf("StartStreamSession returned error: %v", err)
	}

	payload := bytes.Repeat([]byte{1, 2}, 16000)
	if _, err := service.PushStreamChunk(context.Background(), &PushStreamChunkRequest{SessionID: started.SessionID, PCMData: payload}); err != nil {
		t.Fatalf("PushStreamChunk returned error: %v", err)
	}
	if _, err := service.FinishStreamSession(context.Background(), started.SessionID); err != nil {
		t.Fatalf("FinishStreamSession returned error: %v", err)
	}

	result, err := service.CreateTask(context.Background(), 7, &CreateTaskRequest{
		Type:            domain.TaskTypeRealtime,
		ResultText:      "实时文本",
		StreamSessionID: started.SessionID,
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if result == nil || result.ID == 0 {
		t.Fatalf("expected realtime task result, got %+v", result)
	}

	stored, err := repo.GetByID(context.Background(), result.ID)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if stored.LocalFilePath == "" {
		t.Fatalf("expected persisted local audio path, got %+v", stored)
	}
	if stored.Duration <= 0 {
		t.Fatalf("expected positive duration, got %+v", stored)
	}
	if _, err := os.Stat(stored.LocalFilePath); err != nil {
		t.Fatalf("expected materialized audio file, stat err=%v", err)
	}

	if _, err := service.CreateTask(context.Background(), 7, &CreateTaskRequest{
		Type:            domain.TaskTypeRealtime,
		ResultText:      "再次保存",
		StreamSessionID: started.SessionID,
	}); !errors.Is(err, ErrStreamSessionNotFound) {
		t.Fatalf("expected consumed stream session to be removed, got %v", err)
	}

	if err := service.DeleteTask(context.Background(), 7, result.ID); err != nil {
		t.Fatalf("DeleteTask returned error: %v", err)
	}
	if _, err := os.Stat(stored.LocalFilePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected materialized audio file removed, stat err=%v", err)
	}
}

func TestCreateRealtimeTaskRejectsActiveStreamSession(t *testing.T) {
	engine := &streamingBatchEngineServiceStub{startSessionID: "upstream-stream-1"}
	service := NewService(&taskRepoServiceStub{}, engine, nil, 5, nil)
	started, err := service.StartStreamSession(context.Background())
	if err != nil {
		t.Fatalf("StartStreamSession returned error: %v", err)
	}

	_, err = service.CreateTask(context.Background(), 7, &CreateTaskRequest{
		Type:            domain.TaskTypeRealtime,
		ResultText:      "实时文本",
		StreamSessionID: started.SessionID,
	})
	if !errors.Is(err, ErrStreamSessionActive) {
		t.Fatalf("expected ErrStreamSessionActive, got %v", err)
	}
}
