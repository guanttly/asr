package asr

import (
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

func (s *completedTaskProcessorStub) ProcessCompletedTask(_ context.Context, task *domain.TranscriptionTask) error {
	s.processedTask = cloneTask(task)
	return nil
}

func (s *completedTaskProcessorStub) ResumeCompletedTaskFromFailure(_ context.Context, task *domain.TranscriptionTask) error {
	s.resumedTask = cloneTask(task)
	return s.resumeErr
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

func (r *taskRepoServiceStub) ListByUser(_ context.Context, _ uint64, _, _ int) ([]*domain.TranscriptionTask, int64, error) {
	return nil, 0, nil
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
