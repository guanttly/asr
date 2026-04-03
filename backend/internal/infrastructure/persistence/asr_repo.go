package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	domain "github.com/lgt/asr/internal/domain/asr"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const latestRetryOperationKey = "dashboard_latest_post_process_retry"

type retryHistoryPayload struct {
	Items []domain.RetryPostProcessRecord `json:"items"`
}

// TaskModel is the GORM model for transcription_tasks.
type TaskModel struct {
	ID                uint64  `gorm:"primaryKey;autoIncrement"`
	UserID            uint64  `gorm:"index;not null"`
	Type              string  `gorm:"type:varchar(20);not null"`
	Status            string  `gorm:"type:varchar(20);not null;default:'pending'"`
	ExternalTaskID    string  `gorm:"type:varchar(128);index"`
	MeetingID         *uint64 `gorm:"index"`
	PostProcessStatus string  `gorm:"type:varchar(20);not null;default:'pending';index"`
	PostProcessError  string  `gorm:"type:text"`
	PostProcessedAt   *time.Time
	SyncFailCount     int    `gorm:"not null;default:0"`
	LastSyncError     string `gorm:"type:text"`
	LastSyncAt        *time.Time
	NextSyncAt        *time.Time `gorm:"index"`
	AudioURL          string     `gorm:"type:varchar(512)"`
	LocalFilePath     string     `gorm:"type:varchar(1024)"`
	SegmentTotal      int        `gorm:"not null;default:0"`
	SegmentCompleted  int        `gorm:"not null;default:0"`
	ResultText        string     `gorm:"type:longtext"`
	Duration          float64
	DictID            *uint64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type AdminOperationStateModel struct {
	OperationKey string `gorm:"primaryKey;type:varchar(64)"`
	Payload      string `gorm:"type:longtext;not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (TaskModel) TableName() string                { return "transcription_tasks" }
func (AdminOperationStateModel) TableName() string { return "admin_operation_states" }

type TaskRepo struct {
	db *gorm.DB
}

func NewTaskRepo(db *gorm.DB) *TaskRepo {
	return &TaskRepo{db: db}
}

func (r *TaskRepo) Create(ctx context.Context, task *domain.TranscriptionTask) error {
	m := &TaskModel{
		UserID:            task.UserID,
		Type:              string(task.Type),
		Status:            string(task.Status),
		ExternalTaskID:    task.ExternalTaskID,
		MeetingID:         task.MeetingID,
		PostProcessStatus: string(task.PostProcessStatus),
		PostProcessError:  task.PostProcessError,
		PostProcessedAt:   task.PostProcessedAt,
		SyncFailCount:     task.SyncFailCount,
		LastSyncError:     task.LastSyncError,
		LastSyncAt:        task.LastSyncAt,
		NextSyncAt:        task.NextSyncAt,
		AudioURL:          task.AudioURL,
		LocalFilePath:     task.LocalFilePath,
		SegmentTotal:      task.SegmentTotal,
		SegmentCompleted:  task.SegmentCompleted,
		ResultText:        task.ResultText,
		Duration:          task.Duration,
		DictID:            task.DictID,
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return err
	}
	task.ID = m.ID
	task.CreatedAt = m.CreatedAt
	task.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *TaskRepo) GetByID(ctx context.Context, id uint64) (*domain.TranscriptionTask, error) {
	var m TaskModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, err
	}
	return r.toDomain(&m), nil
}

func (r *TaskRepo) Update(ctx context.Context, task *domain.TranscriptionTask) error {
	return r.db.WithContext(ctx).Model(&TaskModel{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":              string(task.Status),
		"external_task_id":    task.ExternalTaskID,
		"meeting_id":          task.MeetingID,
		"post_process_status": string(task.PostProcessStatus),
		"post_process_error":  task.PostProcessError,
		"post_processed_at":   task.PostProcessedAt,
		"sync_fail_count":     task.SyncFailCount,
		"last_sync_error":     task.LastSyncError,
		"last_sync_at":        task.LastSyncAt,
		"next_sync_at":        task.NextSyncAt,
		"local_file_path":     task.LocalFilePath,
		"segment_total":       task.SegmentTotal,
		"segment_completed":   task.SegmentCompleted,
		"result_text":         task.ResultText,
		"duration":            task.Duration,
		"updated_at":          time.Now(),
	}).Error
}

func (r *TaskRepo) ListByUser(ctx context.Context, userID uint64, offset, limit int) ([]*domain.TranscriptionTask, int64, error) {
	var models []TaskModel
	var total int64
	q := r.db.WithContext(ctx).Model(&TaskModel{}).Where("user_id = ?", userID)
	q.Count(&total)
	if err := q.Order("created_at DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}
	tasks := make([]*domain.TranscriptionTask, len(models))
	for i := range models {
		tasks[i] = r.toDomain(&models[i])
	}
	return tasks, total, nil
}

func (r *TaskRepo) ListSyncCandidates(ctx context.Context, limit int) ([]*domain.TranscriptionTask, error) {
	if limit <= 0 {
		limit = 20
	}

	var models []TaskModel
	err := r.db.WithContext(ctx).
		Where("type = ?", string(domain.TaskTypeBatch)).
		Where("((external_task_id <> '' AND (status IN ? OR (status = ? AND post_process_status <> ?))) OR (external_task_id = '' AND status IN ?))",
			[]string{string(domain.TaskStatusPending), string(domain.TaskStatusProcessing)},
			string(domain.TaskStatusCompleted),
			string(domain.PostProcessCompleted),
			[]string{string(domain.TaskStatusPending), string(domain.TaskStatusProcessing)},
		).
		Where("next_sync_at IS NULL OR next_sync_at <= ?", time.Now()).
		Order("updated_at ASC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}

	tasks := make([]*domain.TranscriptionTask, len(models))
	for i := range models {
		tasks[i] = r.toDomain(&models[i])
	}

	return tasks, nil
}

func (r *TaskRepo) ListPostProcessRetryCandidates(ctx context.Context, limit int) ([]*domain.TranscriptionTask, error) {
	if limit <= 0 {
		limit = 20
	}

	var models []TaskModel
	err := r.db.WithContext(ctx).
		Where("type = ?", string(domain.TaskTypeBatch)).
		Where("status = ?", string(domain.TaskStatusCompleted)).
		Where("post_process_status = ?", string(domain.PostProcessFailed)).
		Order("updated_at DESC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}

	tasks := make([]*domain.TranscriptionTask, len(models))
	for i := range models {
		tasks[i] = r.toDomain(&models[i])
	}

	return tasks, nil
}

func (r *TaskRepo) SaveLatestRetryResult(ctx context.Context, record *domain.RetryPostProcessRecord, maxHistory int) error {
	if record == nil {
		return nil
	}
	if maxHistory <= 0 {
		maxHistory = 5
	}

	history, err := r.GetRetryHistory(ctx, maxHistory-1)
	if err != nil {
		return err
	}

	payloadItems := make([]domain.RetryPostProcessRecord, 0, maxHistory)
	payloadItems = append(payloadItems, *record)
	for _, item := range history {
		if item == nil {
			continue
		}
		if len(payloadItems) >= maxHistory {
			break
		}
		payloadItems = append(payloadItems, *item)
	}

	return r.saveRetryHistory(ctx, payloadItems)
}

func (r *TaskRepo) GetLatestRetryResult(ctx context.Context) (*domain.RetryPostProcessRecord, error) {
	history, err := r.GetRetryHistory(ctx, 1)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return nil, nil
	}
	return history[0], nil
}

func (r *TaskRepo) GetRetryHistory(ctx context.Context, limit int) ([]*domain.RetryPostProcessRecord, error) {
	var model AdminOperationStateModel
	if err := r.db.WithContext(ctx).First(&model, "operation_key = ?", latestRetryOperationKey).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	records, err := decodeRetryHistoryPayload(model.Payload)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}

	items := make([]*domain.RetryPostProcessRecord, 0, len(records))
	for i := range records {
		record := records[i]
		items = append(items, &record)
	}

	return items, nil
}

func (r *TaskRepo) ClearRetryHistory(ctx context.Context) error {
	return r.db.WithContext(ctx).Delete(&AdminOperationStateModel{}, "operation_key = ?", latestRetryOperationKey).Error
}

func (r *TaskRepo) DeleteRetryHistoryItem(ctx context.Context, createdAt time.Time) error {
	history, err := r.GetRetryHistory(ctx, 0)
	if err != nil {
		return err
	}
	if len(history) == 0 {
		return nil
	}

	filtered := make([]domain.RetryPostProcessRecord, 0, len(history))
	for _, item := range history {
		if item == nil {
			continue
		}
		if item.CreatedAt.Equal(createdAt) {
			continue
		}
		filtered = append(filtered, *item)
	}

	return r.saveRetryHistory(ctx, filtered)
}

func (r *TaskRepo) saveRetryHistory(ctx context.Context, records []domain.RetryPostProcessRecord) error {
	if len(records) == 0 {
		return r.ClearRetryHistory(ctx)
	}

	payload, err := json.Marshal(retryHistoryPayload{Items: records})
	if err != nil {
		return err
	}

	model := &AdminOperationStateModel{
		OperationKey: latestRetryOperationKey,
		Payload:      string(payload),
	}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "operation_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"payload", "updated_at"}),
		}).
		Create(model).Error
}

func decodeRetryHistoryPayload(raw string) ([]domain.RetryPostProcessRecord, error) {
	if raw == "" {
		return nil, nil
	}

	var history retryHistoryPayload
	if err := json.Unmarshal([]byte(raw), &history); err == nil && len(history.Items) > 0 {
		return history.Items, nil
	}

	var single domain.RetryPostProcessRecord
	if err := json.Unmarshal([]byte(raw), &single); err != nil {
		return nil, err
	}
	if single.CreatedAt.IsZero() && len(single.Items) == 0 && single.Scanned == 0 && single.Updated == 0 && single.Failed == 0 {
		return nil, nil
	}

	return []domain.RetryPostProcessRecord{single}, nil
}

func (r *TaskRepo) GetSyncHealth(ctx context.Context, warnThreshold, alertLimit int) (*domain.SyncHealthOverview, []domain.SyncAlert, error) {
	if warnThreshold <= 0 {
		warnThreshold = 3
	}
	if alertLimit <= 0 {
		alertLimit = 5
	}

	overview := &domain.SyncHealthOverview{}
	countByStatus := func(status domain.TaskStatus) (int64, error) {
		var count int64
		err := r.db.WithContext(ctx).
			Model(&TaskModel{}).
			Where("type = ?", string(domain.TaskTypeBatch)).
			Where("status = ?", string(status)).
			Count(&count).Error
		return count, err
	}
	countByPostProcessStatus := func(status domain.PostProcessStatus) (int64, error) {
		var count int64
		err := r.db.WithContext(ctx).
			Model(&TaskModel{}).
			Where("type = ?", string(domain.TaskTypeBatch)).
			Where("status = ?", string(domain.TaskStatusCompleted)).
			Where("post_process_status = ?", string(status)).
			Count(&count).Error
		return count, err
	}

	var err error
	if overview.PendingCount, err = countByStatus(domain.TaskStatusPending); err != nil {
		return nil, nil, err
	}
	if overview.ProcessingCount, err = countByStatus(domain.TaskStatusProcessing); err != nil {
		return nil, nil, err
	}
	if overview.CompletedCount, err = countByStatus(domain.TaskStatusCompleted); err != nil {
		return nil, nil, err
	}
	if overview.FailedCount, err = countByStatus(domain.TaskStatusFailed); err != nil {
		return nil, nil, err
	}
	if overview.PostProcessPendingCount, err = countByPostProcessStatus(domain.PostProcessPending); err != nil {
		return nil, nil, err
	}
	if overview.PostProcessProcessingCount, err = countByPostProcessStatus(domain.PostProcessProcessing); err != nil {
		return nil, nil, err
	}
	if overview.PostProcessCompletedCount, err = countByPostProcessStatus(domain.PostProcessCompleted); err != nil {
		return nil, nil, err
	}
	if overview.PostProcessFailedCount, err = countByPostProcessStatus(domain.PostProcessFailed); err != nil {
		return nil, nil, err
	}

	if err := r.db.WithContext(ctx).
		Model(&TaskModel{}).
		Where("type = ?", string(domain.TaskTypeBatch)).
		Where("status IN ? OR (status = ? AND post_process_status <> ?)", []string{string(domain.TaskStatusPending), string(domain.TaskStatusProcessing)}, string(domain.TaskStatusCompleted), string(domain.PostProcessCompleted)).
		Where("sync_fail_count >= ?", warnThreshold).
		Count(&overview.RepeatedFailureCount).Error; err != nil {
		return nil, nil, err
	}

	var latestSync sql.NullTime
	if err := r.db.WithContext(ctx).
		Model(&TaskModel{}).
		Select("MAX(last_sync_at)").
		Where("type = ?", string(domain.TaskTypeBatch)).
		Scan(&latestSync).Error; err != nil {
		return nil, nil, err
	}
	if latestSync.Valid {
		overview.LatestSyncAt = &latestSync.Time
	}

	var alertModels []TaskModel
	if err := r.db.WithContext(ctx).
		Where("type = ?", string(domain.TaskTypeBatch)).
		Where("((status IN ? OR (status = ? AND post_process_status <> ?)) AND sync_fail_count >= ?) OR (status = ? AND post_process_status = ?)", []string{string(domain.TaskStatusPending), string(domain.TaskStatusProcessing)}, string(domain.TaskStatusCompleted), string(domain.PostProcessCompleted), warnThreshold, string(domain.TaskStatusCompleted), string(domain.PostProcessFailed)).
		Order("CASE WHEN post_process_status = 'failed' THEN 0 ELSE 1 END ASC").
		Order("sync_fail_count DESC").
		Order("updated_at DESC").
		Limit(alertLimit).
		Find(&alertModels).Error; err != nil {
		return nil, nil, err
	}

	alerts := make([]domain.SyncAlert, len(alertModels))
	for i, model := range alertModels {
		alertReason := domain.SyncAlertReasonRepeatedSyncFailure
		if domain.TaskStatus(model.Status) == domain.TaskStatusCompleted && domain.PostProcessStatus(model.PostProcessStatus) == domain.PostProcessFailed {
			alertReason = domain.SyncAlertReasonPostProcessFailed
		}
		alerts[i] = domain.SyncAlert{
			TaskID:            model.ID,
			ExternalTaskID:    model.ExternalTaskID,
			MeetingID:         model.MeetingID,
			AlertReason:       alertReason,
			Status:            domain.TaskStatus(model.Status),
			PostProcessStatus: domain.PostProcessStatus(model.PostProcessStatus),
			PostProcessError:  model.PostProcessError,
			SyncFailCount:     model.SyncFailCount,
			LastSyncError:     model.LastSyncError,
			LastSyncAt:        model.LastSyncAt,
			NextSyncAt:        model.NextSyncAt,
			UpdatedAt:         model.UpdatedAt,
		}
	}

	return overview, alerts, nil
}

func (r *TaskRepo) toDomain(m *TaskModel) *domain.TranscriptionTask {
	return &domain.TranscriptionTask{
		ID:                m.ID,
		UserID:            m.UserID,
		Type:              domain.TaskType(m.Type),
		Status:            domain.TaskStatus(m.Status),
		ExternalTaskID:    m.ExternalTaskID,
		MeetingID:         m.MeetingID,
		PostProcessStatus: domain.PostProcessStatus(m.PostProcessStatus),
		PostProcessError:  m.PostProcessError,
		PostProcessedAt:   m.PostProcessedAt,
		SyncFailCount:     m.SyncFailCount,
		LastSyncError:     m.LastSyncError,
		LastSyncAt:        m.LastSyncAt,
		NextSyncAt:        m.NextSyncAt,
		AudioURL:          m.AudioURL,
		LocalFilePath:     m.LocalFilePath,
		SegmentTotal:      m.SegmentTotal,
		SegmentCompleted:  m.SegmentCompleted,
		ResultText:        m.ResultText,
		Duration:          m.Duration,
		DictID:            m.DictID,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}
