package persistence

import (
	"context"
	"errors"
	"time"

	domain "github.com/lgt/asr/internal/domain/meetingupload"
	"gorm.io/gorm"
)

// UploadSessionModel persists a resumable meeting upload session.
type UploadSessionModel struct {
	ID            uint64  `gorm:"primaryKey;autoIncrement"`
	UploadID      string  `gorm:"type:varchar(64);uniqueIndex;not null"`
	UserID        uint64  `gorm:"index;not null"`
	MeetingID     *uint64 `gorm:"index"`
	Status        string  `gorm:"type:varchar(20);not null;default:'recording';index"`
	Format        string  `gorm:"type:varchar(32);not null;default:'pcm_s16le_16000_mono'"`
	Filename      string  `gorm:"type:varchar(255)"`
	Title         string  `gorm:"type:varchar(255)"`
	WorkflowID    *uint64
	Language      string  `gorm:"type:varchar(16);not null;default:'auto'"`
	DurationSec   float64 `gorm:"not null;default:0"`
	TotalBytes    int64   `gorm:"not null;default:0"`
	NextIndex     int     `gorm:"not null;default:0"`
	PublicBaseURL string  `gorm:"type:varchar(512)"`
	StartedAt     time.Time
	LastSeenAt    time.Time `gorm:"index"`
	StoppedAt     *time.Time
	CompletedAt   *time.Time
	AbortedAt     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (UploadSessionModel) TableName() string { return "meeting_upload_sessions" }

// UploadSegmentModel persists one chunk of a session's audio.
type UploadSegmentModel struct {
	ID              uint64 `gorm:"primaryKey;autoIncrement"`
	UploadSessionID uint64 `gorm:"index:idx_upload_segment_unique,unique;not null"`
	SegmentIndex    int    `gorm:"index:idx_upload_segment_unique,unique;not null"`
	Path            string `gorm:"type:varchar(1024);not null"`
	Bytes           int64  `gorm:"not null;default:0"`
	DurationSec     float64
	Checksum        string `gorm:"type:varchar(128)"`
	Status          string `gorm:"type:varchar(16);not null;default:'stored'"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (UploadSegmentModel) TableName() string { return "meeting_upload_segments" }

// UploadRepo is the GORM-backed meetingupload.Repository implementation.
type UploadRepo struct {
	db *gorm.DB
}

// NewUploadRepo creates an upload session repository.
func NewUploadRepo(db *gorm.DB) *UploadRepo { return &UploadRepo{db: db} }

func (r *UploadRepo) CreateSession(ctx context.Context, session *domain.UploadSession) error {
	model := uploadSessionToModel(session)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	session.ID = model.ID
	session.CreatedAt = model.CreatedAt
	session.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *UploadRepo) GetSessionByUploadID(ctx context.Context, uploadID string) (*domain.UploadSession, error) {
	var model UploadSessionModel
	err := r.db.WithContext(ctx).Where("upload_id = ?", uploadID).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return uploadSessionToDomain(&model), nil
}

func (r *UploadRepo) GetSessionByID(ctx context.Context, id uint64) (*domain.UploadSession, error) {
	var model UploadSessionModel
	err := r.db.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return uploadSessionToDomain(&model), nil
}

func (r *UploadRepo) UpdateSession(ctx context.Context, session *domain.UploadSession) error {
	return r.db.WithContext(ctx).Model(&UploadSessionModel{}).Where("id = ?", session.ID).Updates(map[string]any{
		"meeting_id":   session.MeetingID,
		"status":       string(session.Status),
		"format":       session.Format,
		"filename":     session.Filename,
		"title":        session.Title,
		"workflow_id":  session.WorkflowID,
		"language":     session.Language,
		"duration_sec": session.DurationSec,
		"total_bytes":  session.TotalBytes,
		"next_index":   session.NextIndex,
		"last_seen_at": session.LastSeenAt,
		"stopped_at":   session.StoppedAt,
		"completed_at": session.CompletedAt,
		"aborted_at":   session.AbortedAt,
		"updated_at":   time.Now(),
	}).Error
}

func (r *UploadRepo) DeleteSession(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&UploadSessionModel{}, id).Error
}

func (r *UploadRepo) CreateSegment(ctx context.Context, segment *domain.UploadSegment) error {
	model := uploadSegmentToModel(segment)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	segment.ID = model.ID
	segment.CreatedAt = model.CreatedAt
	segment.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *UploadRepo) GetSegment(ctx context.Context, sessionID uint64, index int) (*domain.UploadSegment, error) {
	var model UploadSegmentModel
	err := r.db.WithContext(ctx).
		Where("upload_session_id = ? AND segment_index = ?", sessionID, index).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return uploadSegmentToDomain(&model), nil
}

func (r *UploadRepo) ListSegments(ctx context.Context, sessionID uint64) ([]*domain.UploadSegment, error) {
	var models []UploadSegmentModel
	err := r.db.WithContext(ctx).
		Where("upload_session_id = ?", sessionID).
		Order("segment_index ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	segments := make([]*domain.UploadSegment, len(models))
	for i := range models {
		segments[i] = uploadSegmentToDomain(&models[i])
	}
	return segments, nil
}

func (r *UploadRepo) DeleteSegments(ctx context.Context, sessionID uint64) error {
	return r.db.WithContext(ctx).
		Where("upload_session_id = ?", sessionID).
		Delete(&UploadSegmentModel{}).Error
}

func (r *UploadRepo) ListStaleRecording(ctx context.Context, lastSeenBefore time.Time, limit int) ([]*domain.UploadSession, error) {
	if limit <= 0 {
		limit = 50
	}
	var models []UploadSessionModel
	err := r.db.WithContext(ctx).
		Where("status = ?", string(domain.SessionStatusRecording)).
		Where("last_seen_at < ?", lastSeenBefore).
		Order("last_seen_at ASC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	return uploadSessionsToDomain(models), nil
}

func (r *UploadRepo) ListRecoverable(ctx context.Context, statuses []domain.SessionStatus, limit int) ([]*domain.UploadSession, error) {
	if limit <= 0 {
		limit = 50
	}
	raw := make([]string, len(statuses))
	for i, s := range statuses {
		raw[i] = string(s)
	}
	var models []UploadSessionModel
	err := r.db.WithContext(ctx).
		Where("status IN ?", raw).
		Order("last_seen_at ASC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	return uploadSessionsToDomain(models), nil
}

func (r *UploadRepo) ListCleanupCandidates(ctx context.Context, q domain.CleanupQuery, limit int) ([]*domain.UploadSession, error) {
	if limit <= 0 {
		limit = 100
	}

	var orGroup *gorm.DB
	add := func(cond *gorm.DB) {
		if orGroup == nil {
			orGroup = cond
		} else {
			orGroup = orGroup.Or(cond)
		}
	}
	if !q.AbortedBefore.IsZero() {
		add(r.db.Where("status = ? AND updated_at < ?", string(domain.SessionStatusAborted), q.AbortedBefore))
	}
	if !q.CompletedBefore.IsZero() {
		add(r.db.Where("status = ? AND completed_at IS NOT NULL AND completed_at < ?", string(domain.SessionStatusCompleted), q.CompletedBefore))
	}
	if !q.FailedBefore.IsZero() {
		add(r.db.Where("status = ? AND updated_at < ?", string(domain.SessionStatusFailed), q.FailedBefore))
	}
	if !q.InterruptedBefore.IsZero() {
		add(r.db.Where("status = ? AND last_seen_at < ?", string(domain.SessionStatusInterrupted), q.InterruptedBefore))
	}
	if orGroup == nil {
		return nil, nil
	}

	var models []UploadSessionModel
	err := r.db.WithContext(ctx).Model(&UploadSessionModel{}).
		Where(orGroup).
		Order("updated_at ASC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	return uploadSessionsToDomain(models), nil
}

func uploadSessionsToDomain(models []UploadSessionModel) []*domain.UploadSession {
	sessions := make([]*domain.UploadSession, len(models))
	for i := range models {
		sessions[i] = uploadSessionToDomain(&models[i])
	}
	return sessions
}

func uploadSessionToModel(s *domain.UploadSession) *UploadSessionModel {
	return &UploadSessionModel{
		ID:            s.ID,
		UploadID:      s.UploadID,
		UserID:        s.UserID,
		MeetingID:     s.MeetingID,
		Status:        string(s.Status),
		Format:        s.Format,
		Filename:      s.Filename,
		Title:         s.Title,
		WorkflowID:    s.WorkflowID,
		Language:      s.Language,
		DurationSec:   s.DurationSec,
		TotalBytes:    s.TotalBytes,
		NextIndex:     s.NextIndex,
		PublicBaseURL: s.PublicBaseURL,
		StartedAt:     s.StartedAt,
		LastSeenAt:    s.LastSeenAt,
		StoppedAt:     s.StoppedAt,
		CompletedAt:   s.CompletedAt,
		AbortedAt:     s.AbortedAt,
	}
}

func uploadSessionToDomain(m *UploadSessionModel) *domain.UploadSession {
	return &domain.UploadSession{
		ID:            m.ID,
		UploadID:      m.UploadID,
		UserID:        m.UserID,
		MeetingID:     m.MeetingID,
		Status:        domain.SessionStatus(m.Status),
		Format:        m.Format,
		Filename:      m.Filename,
		Title:         m.Title,
		WorkflowID:    m.WorkflowID,
		Language:      m.Language,
		DurationSec:   m.DurationSec,
		TotalBytes:    m.TotalBytes,
		NextIndex:     m.NextIndex,
		PublicBaseURL: m.PublicBaseURL,
		StartedAt:     m.StartedAt,
		LastSeenAt:    m.LastSeenAt,
		StoppedAt:     m.StoppedAt,
		CompletedAt:   m.CompletedAt,
		AbortedAt:     m.AbortedAt,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

func uploadSegmentToModel(s *domain.UploadSegment) *UploadSegmentModel {
	return &UploadSegmentModel{
		ID:              s.ID,
		UploadSessionID: s.UploadSessionID,
		SegmentIndex:    s.SegmentIndex,
		Path:            s.Path,
		Bytes:           s.Bytes,
		DurationSec:     s.DurationSec,
		Checksum:        s.Checksum,
		Status:          string(s.Status),
	}
}

func uploadSegmentToDomain(m *UploadSegmentModel) *domain.UploadSegment {
	return &domain.UploadSegment{
		ID:              m.ID,
		UploadSessionID: m.UploadSessionID,
		SegmentIndex:    m.SegmentIndex,
		Path:            m.Path,
		Bytes:           m.Bytes,
		DurationSec:     m.DurationSec,
		Checksum:        m.Checksum,
		Status:          domain.SegmentStatus(m.Status),
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}
