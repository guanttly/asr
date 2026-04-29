package asr

import (
	"fmt"
	"math"
	"strings"
	"time"

	domain "github.com/lgt/asr/internal/domain/asr"
)

// CreateTaskRequest is the DTO for creating a new transcription task.
type CreateTaskRequest struct {
	AudioURL        string          `json:"audio_url"`
	StreamSessionID string          `json:"stream_session_id,omitempty"`
	LocalFilePath   string          `json:"-"`
	Type            domain.TaskType `json:"type" binding:"required,oneof=realtime batch"`
	DictID          *uint64         `json:"dict_id"`
	WorkflowID      *uint64         `json:"workflow_id"`
	ResultText      string          `json:"result_text"`
	Duration        float64         `json:"duration"`
}

// TranscribeSnippetRequest is the DTO for one-shot short-segment recognition.
type TranscribeSnippetRequest struct {
	LocalFilePath string  `json:"-"`
	DictID        *uint64 `json:"dict_id"`
}

// TranscribeSnippetResponse is the DTO returned by the short-segment endpoint.
type TranscribeSnippetResponse struct {
	Status   string  `json:"status"`
	Text     string  `json:"text"`
	Duration float64 `json:"duration"`
}

// PushStreamChunkRequest is the DTO for pushing one PCM chunk into a streaming session.
type PushStreamChunkRequest struct {
	SessionID string `json:"session_id"`
	PCMData   []byte `json:"-"`
}

// StreamSessionResponse reports the backend-managed upstream streaming session id.
type StreamSessionResponse struct {
	SessionID string `json:"session_id"`
}

// StreamChunkResponse is the normalized incremental/final streaming ASR result.
type StreamChunkResponse struct {
	SessionID string `json:"session_id,omitempty"`
	Language  string `json:"language,omitempty"`
	Text      string `json:"text"`
	TextDelta string `json:"text_delta,omitempty"`
	IsFinal   bool   `json:"is_final,omitempty"`
}

// StreamSessionState is a read-only snapshot of one managed streaming session.
type StreamSessionState struct {
	SessionID     string    `json:"session_id"`
	Language      string    `json:"language,omitempty"`
	Text          string    `json:"text,omitempty"`
	CommittedText string    `json:"committed_text,omitempty"`
	Duration      float64   `json:"duration,omitempty"`
	IsFinal       bool      `json:"is_final,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
}

// TaskResponse is the DTO returned to clients.
type TaskResponse struct {
	ID                uint64                   `json:"id"`
	Type              domain.TaskType          `json:"type"`
	Status            domain.TaskStatus        `json:"status"`
	ExternalTaskID    string                   `json:"external_task_id,omitempty"`
	ProgressPercent   int                      `json:"progress_percent"`
	ProgressStage     string                   `json:"progress_stage,omitempty"`
	ProgressMessage   string                   `json:"progress_message,omitempty"`
	SegmentTotal      int                      `json:"segment_total,omitempty"`
	SegmentCompleted  int                      `json:"segment_completed,omitempty"`
	AudioURL          string                   `json:"audio_url,omitempty"`
	MeetingID         *uint64                  `json:"meeting_id,omitempty"`
	PostProcessStatus domain.PostProcessStatus `json:"post_process_status"`
	PostProcessError  string                   `json:"post_process_error,omitempty"`
	PostProcessedAt   *time.Time               `json:"post_processed_at,omitempty"`
	SyncFailCount     int                      `json:"sync_fail_count"`
	LastSyncError     string                   `json:"last_sync_error,omitempty"`
	LastSyncAt        *time.Time               `json:"last_sync_at,omitempty"`
	NextSyncAt        *time.Time               `json:"next_sync_at,omitempty"`
	ResultText        string                   `json:"result_text,omitempty"`
	Duration          float64                  `json:"duration"`
	WorkflowID        *uint64                  `json:"workflow_id,omitempty"`
	CreatedAt         time.Time                `json:"created_at"`
	UpdatedAt         time.Time                `json:"updated_at"`
}

// TaskListResponse wraps a paginated list of tasks.
type TaskListResponse struct {
	Items []*TaskResponse `json:"items"`
	Total int64           `json:"total"`
}

// ClearTasksResponse reports how many history records were deleted in one clear action.
type ClearTasksResponse struct {
	DeletedCount int `json:"deleted_count"`
	SkippedCount int `json:"skipped_count"`
}

// SyncHealthResponse is the admin dashboard view of batch task synchronization health.
type SyncHealthResponse struct {
	PendingCount               int64                      `json:"pending_count"`
	ProcessingCount            int64                      `json:"processing_count"`
	CompletedCount             int64                      `json:"completed_count"`
	FailedCount                int64                      `json:"failed_count"`
	PostProcessPendingCount    int64                      `json:"post_process_pending_count"`
	PostProcessProcessingCount int64                      `json:"post_process_processing_count"`
	PostProcessCompletedCount  int64                      `json:"post_process_completed_count"`
	PostProcessFailedCount     int64                      `json:"post_process_failed_count"`
	RepeatedFailureCount       int64                      `json:"repeated_failure_count"`
	LatestSyncAt               *time.Time                 `json:"latest_sync_at,omitempty"`
	LatestRetryResult          *RetryPostProcessResponse  `json:"latest_retry_result,omitempty"`
	RetryHistory               []RetryPostProcessResponse `json:"retry_history,omitempty"`
	Alerts                     []SyncAlertResponse        `json:"alerts"`
}

// SyncAlertResponse is the dashboard DTO for operational risk alerts.
type SyncAlertResponse struct {
	TaskID            uint64                   `json:"task_id"`
	ExternalTaskID    string                   `json:"external_task_id"`
	MeetingID         *uint64                  `json:"meeting_id,omitempty"`
	AlertReason       domain.SyncAlertReason   `json:"alert_reason"`
	Status            domain.TaskStatus        `json:"status"`
	PostProcessStatus domain.PostProcessStatus `json:"post_process_status"`
	PostProcessError  string                   `json:"post_process_error,omitempty"`
	SyncFailCount     int                      `json:"sync_fail_count"`
	LastSyncError     string                   `json:"last_sync_error,omitempty"`
	LastSyncAt        *time.Time               `json:"last_sync_at,omitempty"`
	NextSyncAt        *time.Time               `json:"next_sync_at,omitempty"`
	UpdatedAt         time.Time                `json:"updated_at"`
}

// RetryPostProcessResponse is the batch retry summary returned to the dashboard.
type RetryPostProcessResponse struct {
	Limit              int                            `json:"limit"`
	RequestedTaskCount int                            `json:"requested_task_count"`
	Scanned            int                            `json:"scanned"`
	Updated            int                            `json:"updated"`
	Failed             int                            `json:"failed"`
	CreatedAt          *time.Time                     `json:"created_at,omitempty"`
	Items              []RetryPostProcessItemResponse `json:"items"`
}

// RetryPostProcessItemResponse is one retry attempt result.
type RetryPostProcessItemResponse struct {
	TaskID            uint64                   `json:"task_id"`
	ExternalTaskID    string                   `json:"external_task_id"`
	MeetingID         *uint64                  `json:"meeting_id,omitempty"`
	Outcome           string                   `json:"outcome"`
	PostProcessStatus domain.PostProcessStatus `json:"post_process_status"`
	ErrorMessage      string                   `json:"error_message,omitempty"`
}

// RetryPostProcessRequest controls the batch retry size from the dashboard.
type RetryPostProcessRequest struct {
	Limit   int      `json:"limit" binding:"omitempty,min=1,max=100"`
	TaskIDs []uint64 `json:"task_ids"`
}

// ClearRetryHistoryResponse reports whether dashboard retry history was cleared.
type ClearRetryHistoryResponse struct {
	Cleared bool `json:"cleared"`
}

// DeleteRetryHistoryItemRequest identifies one persisted retry-history record.
type DeleteRetryHistoryItemRequest struct {
	CreatedAt string `json:"created_at" binding:"required"`
}

// DeleteRetryHistoryItemResponse reports whether a history record was removed.
type DeleteRetryHistoryItemResponse struct {
	Deleted bool `json:"deleted"`
}

// ToResponse converts a domain entity to a DTO.
func ToResponse(t *domain.TranscriptionTask) *TaskResponse {
	progressPercent, progressStage, progressMessage := taskProgress(t)
	resultText := sanitizeTranscriptionText(t.ResultText)
	return &TaskResponse{
		ID:                t.ID,
		Type:              t.Type,
		Status:            t.Status,
		ExternalTaskID:    t.ExternalTaskID,
		ProgressPercent:   progressPercent,
		ProgressStage:     progressStage,
		ProgressMessage:   progressMessage,
		SegmentTotal:      t.SegmentTotal,
		SegmentCompleted:  t.SegmentCompleted,
		AudioURL:          t.AudioURL,
		MeetingID:         t.MeetingID,
		PostProcessStatus: t.PostProcessStatus,
		PostProcessError:  t.PostProcessError,
		PostProcessedAt:   t.PostProcessedAt,
		SyncFailCount:     t.SyncFailCount,
		LastSyncError:     t.LastSyncError,
		LastSyncAt:        t.LastSyncAt,
		NextSyncAt:        t.NextSyncAt,
		ResultText:        resultText,
		Duration:          t.Duration,
		WorkflowID:        t.WorkflowID,
		CreatedAt:         t.CreatedAt,
		UpdatedAt:         t.UpdatedAt,
	}
}

func taskProgress(t *domain.TranscriptionTask) (int, string, string) {
	if t == nil {
		return 0, "queued", "任务已入队"
	}
	if t.SegmentTotal > 0 {
		return splitTaskProgress(t)
	}

	switch t.Status {
	case domain.TaskStatusFailed:
		message := t.LastSyncError
		if message == "" {
			message = t.ResultText
		}
		if message == "" {
			message = "任务处理失败"
		}
		return 100, "failed", message
	case domain.TaskStatusCompleted:
		switch t.PostProcessStatus {
		case domain.PostProcessCompleted:
			return 100, "completed", "转写与后处理已完成"
		case domain.PostProcessFailed:
			return 95, "postprocess_failed", "转写完成，后处理失败"
		case domain.PostProcessProcessing:
			return 92, "postprocessing", "正在生成会议记录与摘要"
		default:
			return 85, "transcribed", "转写完成，等待后处理"
		}
	case domain.TaskStatusProcessing:
		if t.ExternalTaskID == "" && strings.TrimSpace(t.ResultText) == "" {
			if t.SyncFailCount > 0 {
				return 15, "retry_waiting", "提交失败，等待后台重试"
			}
			return 20, "submitting", "任务正在提交到 ASR"
		}
		if t.SyncFailCount > 0 {
			return 60, "processing", "ASR 处理中，后台正在重试同步"
		}
		return 60, "processing", "ASR 正在解析音频"
	default:
		if t.SyncFailCount > 0 {
			return 15, "retry_waiting", "提交失败，等待后台重试"
		}
		if t.ExternalTaskID != "" {
			return 25, "submitted", "任务已提交，等待 ASR 调度"
		}
		return 10, "queued", "任务已入队，等待提交到 ASR"
	}
}

func splitTaskProgress(t *domain.TranscriptionTask) (int, string, string) {
	total := t.SegmentTotal
	completed := t.SegmentCompleted
	if completed < 0 {
		completed = 0
	}
	if completed > total {
		completed = total
	}

	percent := segmentProgressPercent(completed, total)
	progressText := fmt.Sprintf("%d/%d", completed, total)

	switch t.Status {
	case domain.TaskStatusFailed:
		message := t.LastSyncError
		if message == "" {
			message = t.ResultText
		}
		if message == "" {
			message = fmt.Sprintf("音频分片转写失败（%s）", progressText)
		} else {
			message = fmt.Sprintf("音频分片转写失败（%s）：%s", progressText, message)
		}
		return percent, "failed", message
	case domain.TaskStatusCompleted:
		switch t.PostProcessStatus {
		case domain.PostProcessCompleted:
			return 100, "completed", fmt.Sprintf("音频分片转写与后处理已完成（%s）", progressText)
		case domain.PostProcessFailed:
			return 100, "postprocess_failed", fmt.Sprintf("音频分片转写完成（%s），后处理失败", progressText)
		case domain.PostProcessProcessing:
			return 100, "postprocessing", fmt.Sprintf("音频分片转写完成（%s），正在生成会议记录与摘要", progressText)
		default:
			return 100, "transcribed", fmt.Sprintf("音频分片转写完成（%s），等待后处理", progressText)
		}
	default:
		currentSegment := completed + 1
		if currentSegment > total {
			currentSegment = total
		}
		currentProgressText := fmt.Sprintf("第 %d/%d 片", currentSegment, total)
		if t.SyncFailCount > 0 {
			return percent, "retry_waiting", fmt.Sprintf("%s处理中断，等待后台重试（已完成 %s）", currentProgressText, progressText)
		}
		if completed >= total {
			return 100, "processing", fmt.Sprintf("音频分片转写完成，正在汇总结果（%s）", progressText)
		}
		return percent, "processing", fmt.Sprintf("当前%s处理中（已完成 %s）", currentProgressText, progressText)
	}
}

func segmentProgressPercent(completed, total int) int {
	if total <= 0 {
		return 0
	}
	if completed < 0 {
		completed = 0
	}
	if completed > total {
		completed = total
	}
	return int(math.Round(float64(completed) * 100 / float64(total)))
}
