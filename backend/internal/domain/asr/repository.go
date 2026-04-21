package asr

import (
	"context"
	"time"
)

// SyncHealthOverview aggregates batch transcription sync health signals.
type SyncHealthOverview struct {
	PendingCount               int64
	ProcessingCount            int64
	CompletedCount             int64
	FailedCount                int64
	PostProcessPendingCount    int64
	PostProcessProcessingCount int64
	PostProcessCompletedCount  int64
	PostProcessFailedCount     int64
	RepeatedFailureCount       int64
	LatestSyncAt               *time.Time
}

// RetryPostProcessRecord stores the latest batch retry result for dashboard recovery.
type RetryPostProcessRecord struct {
	Limit              int
	RequestedTaskCount int
	Scanned            int
	Updated            int
	Failed             int
	CreatedAt          time.Time
	Items              []RetryPostProcessRecordItem
}

// RetryPostProcessRecordItem stores one task result inside a retry batch.
type RetryPostProcessRecordItem struct {
	TaskID            uint64
	ExternalTaskID    string
	MeetingID         *uint64
	Outcome           string
	PostProcessStatus PostProcessStatus
	ErrorMessage      string
}

// SyncAlertReason marks why a task appears in the dashboard alert list.
type SyncAlertReason string

const (
	SyncAlertReasonRepeatedSyncFailure SyncAlertReason = "repeated_sync_failure"
	SyncAlertReasonPostProcessFailed   SyncAlertReason = "post_process_failed"
)

// SyncAlert describes a task that needs operator attention.
type SyncAlert struct {
	TaskID            uint64
	ExternalTaskID    string
	MeetingID         *uint64
	AlertReason       SyncAlertReason
	Status            TaskStatus
	PostProcessStatus PostProcessStatus
	PostProcessError  string
	SyncFailCount     int
	LastSyncError     string
	LastSyncAt        *time.Time
	NextSyncAt        *time.Time
	UpdatedAt         time.Time
}

// TaskRepository defines persistence operations for TranscriptionTask.
type TaskRepository interface {
	Create(ctx context.Context, task *TranscriptionTask) error
	GetByID(ctx context.Context, id uint64) (*TranscriptionTask, error)
	Update(ctx context.Context, task *TranscriptionTask) error
	Delete(ctx context.Context, id uint64) error
	ListByUser(ctx context.Context, userID uint64, taskType *TaskType, offset, limit int) ([]*TranscriptionTask, int64, error)
	ListSyncCandidates(ctx context.Context, limit int) ([]*TranscriptionTask, error)
	ListPostProcessRetryCandidates(ctx context.Context, limit int) ([]*TranscriptionTask, error)
	SaveLatestRetryResult(ctx context.Context, record *RetryPostProcessRecord, maxHistory int) error
	GetLatestRetryResult(ctx context.Context) (*RetryPostProcessRecord, error)
	GetRetryHistory(ctx context.Context, limit int) ([]*RetryPostProcessRecord, error)
	DeleteRetryHistoryItem(ctx context.Context, createdAt time.Time) error
	ClearRetryHistory(ctx context.Context) error
	GetSyncHealth(ctx context.Context, warnThreshold, alertLimit int) (*SyncHealthOverview, []SyncAlert, error)
}
