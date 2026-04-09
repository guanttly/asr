package asr

import "time"

// TaskStatus represents the lifecycle state of a transcription task.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
)

// TaskType distinguishes realtime streaming from batch file upload.
type TaskType string

const (
	TaskTypeRealtime TaskType = "realtime"
	TaskTypeBatch    TaskType = "batch"
)

// PostProcessStatus represents downstream meeting/summary materialization state.
type PostProcessStatus string

const (
	PostProcessPending    PostProcessStatus = "pending"
	PostProcessProcessing PostProcessStatus = "processing"
	PostProcessCompleted  PostProcessStatus = "completed"
	PostProcessFailed     PostProcessStatus = "failed"
)

// TranscriptionTask is the core aggregate for a transcription job.
type TranscriptionTask struct {
	ID                uint64            `json:"id"`
	UserID            uint64            `json:"user_id"`
	Type              TaskType          `json:"type"`
	Status            TaskStatus        `json:"status"`
	ExternalTaskID    string            `json:"external_task_id"`
	MeetingID         *uint64           `json:"meeting_id,omitempty"`
	PostProcessStatus PostProcessStatus `json:"post_process_status"`
	PostProcessError  string            `json:"post_process_error"`
	PostProcessedAt   *time.Time        `json:"post_processed_at,omitempty"`
	SyncFailCount     int               `json:"sync_fail_count"`
	LastSyncError     string            `json:"last_sync_error"`
	LastSyncAt        *time.Time        `json:"last_sync_at,omitempty"`
	NextSyncAt        *time.Time        `json:"next_sync_at,omitempty"`
	AudioURL          string            `json:"audio_url"`
	LocalFilePath     string            `json:"-"`
	SegmentTotal      int               `json:"segment_total"`
	SegmentCompleted  int               `json:"segment_completed"`
	ResultText        string            `json:"result_text"`
	Duration          float64           `json:"duration"`    // audio duration in seconds
	DictID            *uint64           `json:"dict_id"`     // optional terminology dict
	WorkflowID        *uint64           `json:"workflow_id"` // optional workflow for post-processing
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// CanTransition checks whether a status transition is valid.
func (t *TranscriptionTask) CanTransition(to TaskStatus) bool {
	switch t.Status {
	case TaskStatusPending:
		return to == TaskStatusProcessing || to == TaskStatusCompleted || to == TaskStatusFailed
	case TaskStatusProcessing:
		return to == TaskStatusCompleted || to == TaskStatusFailed
	default:
		return false
	}
}

// TransitionTo attempts to move the task to a new status.
func (t *TranscriptionTask) TransitionTo(to TaskStatus) bool {
	if !t.CanTransition(to) {
		return false
	}
	t.Status = to
	t.UpdatedAt = time.Now()
	return true
}
