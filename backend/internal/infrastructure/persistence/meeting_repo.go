package persistence

import (
	"context"
	"time"

	domain "github.com/lgt/asr/internal/domain/meeting"
	"gorm.io/gorm"
)

// MeetingModel is the persistence model for meetings.
type MeetingModel struct {
	ID           uint64  `gorm:"primaryKey;autoIncrement"`
	SourceTaskID *uint64 `gorm:"uniqueIndex"`
	UserID       uint64  `gorm:"index;not null"`
	Title        string  `gorm:"type:varchar(255);not null"`
	AudioURL     string  `gorm:"type:varchar(512);not null"`
	Duration     float64
	Status       string `gorm:"type:varchar(20);not null;default:'uploaded'"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (MeetingModel) TableName() string { return "meetings" }

// TranscriptModel is the persistence model for meeting transcripts.
type TranscriptModel struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	MeetingID    uint64 `gorm:"index;not null"`
	SpeakerLabel string `gorm:"type:varchar(64);not null"`
	Text         string `gorm:"type:longtext;not null"`
	StartTime    float64
	EndTime      float64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (TranscriptModel) TableName() string { return "meeting_transcripts" }

// SummaryModel is the persistence model for generated summaries.
type SummaryModel struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	MeetingID    uint64 `gorm:"uniqueIndex;not null"`
	Content      string `gorm:"type:longtext;not null"`
	ModelVersion string `gorm:"type:varchar(64);not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (SummaryModel) TableName() string { return "meeting_summaries" }

type MeetingRepo struct {
	db *gorm.DB
}

func NewMeetingRepo(db *gorm.DB) *MeetingRepo {
	return &MeetingRepo{db: db}
}

func (r *MeetingRepo) Create(ctx context.Context, meeting *domain.Meeting) error {
	model := &MeetingModel{
		SourceTaskID: meeting.SourceTaskID,
		UserID:       meeting.UserID,
		Title:        meeting.Title,
		AudioURL:     meeting.AudioURL,
		Duration:     meeting.Duration,
		Status:       string(meeting.Status),
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	meeting.ID = model.ID
	meeting.CreatedAt = model.CreatedAt
	meeting.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *MeetingRepo) GetBySourceTaskID(ctx context.Context, sourceTaskID uint64) (*domain.Meeting, error) {
	var model MeetingModel
	err := r.db.WithContext(ctx).Where("source_task_id = ?", sourceTaskID).First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *MeetingRepo) GetByID(ctx context.Context, id uint64) (*domain.Meeting, error) {
	var model MeetingModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *MeetingRepo) Update(ctx context.Context, meeting *domain.Meeting) error {
	return r.db.WithContext(ctx).Model(&MeetingModel{}).Where("id = ?", meeting.ID).Updates(map[string]any{
		"source_task_id": meeting.SourceTaskID,
		"title":          meeting.Title,
		"status":         string(meeting.Status),
		"duration":       meeting.Duration,
		"updated_at":     time.Now(),
	}).Error
}

func (r *MeetingRepo) List(ctx context.Context, userID uint64, offset, limit int) ([]*domain.Meeting, int64, error) {
	var models []MeetingModel
	var total int64
	query := r.db.WithContext(ctx).Model(&MeetingModel{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("created_at desc").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}
	items := make([]*domain.Meeting, len(models))
	for i := range models {
		items[i] = r.toDomain(&models[i])
	}
	return items, total, nil
}

func (r *MeetingRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&MeetingModel{}, id).Error
}

func (r *MeetingRepo) toDomain(model *MeetingModel) *domain.Meeting {
	return &domain.Meeting{
		ID:           model.ID,
		SourceTaskID: model.SourceTaskID,
		UserID:       model.UserID,
		Title:        model.Title,
		AudioURL:     model.AudioURL,
		Duration:     model.Duration,
		Status:       domain.MeetingStatus(model.Status),
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}

type TranscriptRepo struct {
	db *gorm.DB
}

func NewTranscriptRepo(db *gorm.DB) *TranscriptRepo {
	return &TranscriptRepo{db: db}
}

func (r *TranscriptRepo) BatchCreate(ctx context.Context, transcripts []domain.Transcript) error {
	models := make([]TranscriptModel, len(transcripts))
	for i, transcript := range transcripts {
		models[i] = TranscriptModel{
			MeetingID:    transcript.MeetingID,
			SpeakerLabel: transcript.SpeakerLabel,
			Text:         transcript.Text,
			StartTime:    transcript.StartTime,
			EndTime:      transcript.EndTime,
		}
	}
	return r.db.WithContext(ctx).Create(&models).Error
}

func (r *TranscriptRepo) ListByMeeting(ctx context.Context, meetingID uint64) ([]domain.Transcript, error) {
	var models []TranscriptModel
	if err := r.db.WithContext(ctx).Where("meeting_id = ?", meetingID).Order("start_time asc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]domain.Transcript, len(models))
	for i, model := range models {
		items[i] = domain.Transcript{
			ID:           model.ID,
			MeetingID:    model.MeetingID,
			SpeakerLabel: model.SpeakerLabel,
			Text:         model.Text,
			StartTime:    model.StartTime,
			EndTime:      model.EndTime,
		}
	}
	return items, nil
}

type SummaryRepo struct {
	db *gorm.DB
}

func NewSummaryRepo(db *gorm.DB) *SummaryRepo {
	return &SummaryRepo{db: db}
}

func (r *SummaryRepo) Create(ctx context.Context, summary *domain.Summary) error {
	model := &SummaryModel{
		MeetingID:    summary.MeetingID,
		Content:      summary.Content,
		ModelVersion: summary.ModelVersion,
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	summary.ID = model.ID
	summary.CreatedAt = model.CreatedAt
	return nil
}

func (r *SummaryRepo) GetByMeeting(ctx context.Context, meetingID uint64) (*domain.Summary, error) {
	var model SummaryModel
	if err := r.db.WithContext(ctx).Where("meeting_id = ?", meetingID).First(&model).Error; err != nil {
		return nil, err
	}
	return &domain.Summary{
		ID:           model.ID,
		MeetingID:    model.MeetingID,
		Content:      model.Content,
		ModelVersion: model.ModelVersion,
		CreatedAt:    model.CreatedAt,
	}, nil
}

func (r *SummaryRepo) Update(ctx context.Context, summary *domain.Summary) error {
	return r.db.WithContext(ctx).Model(&SummaryModel{}).Where("id = ?", summary.ID).Updates(map[string]any{
		"content":       summary.Content,
		"model_version": summary.ModelVersion,
		"updated_at":    time.Now(),
	}).Error
}
