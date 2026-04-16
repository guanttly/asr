package persistence

import (
	"context"
	"time"

	domain "github.com/lgt/asr/internal/domain/sensitive"
	"gorm.io/gorm"
)

type SensitiveDictModel struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	Name        string `gorm:"type:varchar(128);not null"`
	Scene       string `gorm:"type:varchar(128);index;not null"`
	Description string `gorm:"type:text"`
	IsBase      bool   `gorm:"not null;default:false;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (SensitiveDictModel) TableName() string { return "sensitive_dicts" }

type SensitiveEntryModel struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	DictID    uint64 `gorm:"index;not null"`
	Word      string `gorm:"type:varchar(255);not null"`
	Enabled   bool   `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (SensitiveEntryModel) TableName() string { return "sensitive_entries" }

type SensitiveDictRepo struct{ db *gorm.DB }

func NewSensitiveDictRepo(db *gorm.DB) *SensitiveDictRepo { return &SensitiveDictRepo{db: db} }

func (r *SensitiveDictRepo) Create(ctx context.Context, dict *domain.Dict) error {
	model := &SensitiveDictModel{Name: dict.Name, Scene: dict.Scene, Description: dict.Description, IsBase: dict.IsBase}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	dict.ID = model.ID
	dict.CreatedAt = model.CreatedAt
	dict.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *SensitiveDictRepo) GetByID(ctx context.Context, id uint64) (*domain.Dict, error) {
	var model SensitiveDictModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return &domain.Dict{ID: model.ID, Name: model.Name, Scene: model.Scene, Description: model.Description, IsBase: model.IsBase, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}, nil
}

func (r *SensitiveDictRepo) Update(ctx context.Context, dict *domain.Dict) error {
	return r.db.WithContext(ctx).Model(&SensitiveDictModel{}).Where("id = ?", dict.ID).Updates(map[string]any{
		"name": dict.Name, "scene": dict.Scene, "description": dict.Description, "is_base": dict.IsBase, "updated_at": time.Now(),
	}).Error
}

func (r *SensitiveDictRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&SensitiveDictModel{}, id).Error
}

func (r *SensitiveDictRepo) List(ctx context.Context, offset, limit int) ([]*domain.Dict, int64, error) {
	var models []SensitiveDictModel
	var total int64
	query := r.db.WithContext(ctx).Model(&SensitiveDictModel{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("is_base desc, created_at desc").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}
	items := make([]*domain.Dict, len(models))
	for i, model := range models {
		items[i] = &domain.Dict{ID: model.ID, Name: model.Name, Scene: model.Scene, Description: model.Description, IsBase: model.IsBase, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
	}
	return items, total, nil
}

type SensitiveEntryRepo struct{ db *gorm.DB }

func NewSensitiveEntryRepo(db *gorm.DB) *SensitiveEntryRepo { return &SensitiveEntryRepo{db: db} }

func (r *SensitiveEntryRepo) Create(ctx context.Context, entry *domain.Entry) error {
	model := &SensitiveEntryModel{DictID: entry.DictID, Word: entry.Word, Enabled: entry.Enabled}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	entry.ID = model.ID
	entry.CreatedAt = model.CreatedAt
	entry.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *SensitiveEntryRepo) GetByID(ctx context.Context, id uint64) (*domain.Entry, error) {
	var model SensitiveEntryModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return &domain.Entry{ID: model.ID, DictID: model.DictID, Word: model.Word, Enabled: model.Enabled, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}, nil
}

func (r *SensitiveEntryRepo) ListByDict(ctx context.Context, dictID uint64) ([]domain.Entry, error) {
	var models []SensitiveEntryModel
	if err := r.db.WithContext(ctx).Where("dict_id = ?", dictID).Order("id asc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]domain.Entry, len(models))
	for i, model := range models {
		items[i] = domain.Entry{ID: model.ID, DictID: model.DictID, Word: model.Word, Enabled: model.Enabled, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
	}
	return items, nil
}

func (r *SensitiveEntryRepo) ListAppliedByDict(ctx context.Context, dictID uint64) ([]domain.Entry, error) {
	query := r.db.WithContext(ctx).
		Table("sensitive_entries AS e").
		Select("e.id, e.dict_id, e.word, e.enabled, e.created_at, e.updated_at").
		Joins("JOIN sensitive_dicts AS d ON d.id = e.dict_id").
		Where("e.enabled = ?", true)
	if dictID > 0 {
		query = query.Where("d.is_base = ? OR d.id = ?", true, dictID)
	} else {
		query = query.Where("d.is_base = ?", true)
	}
	var models []SensitiveEntryModel
	if err := query.Order("CHAR_LENGTH(e.word) desc, e.id asc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]domain.Entry, len(models))
	for i, model := range models {
		items[i] = domain.Entry{ID: model.ID, DictID: model.DictID, Word: model.Word, Enabled: model.Enabled, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
	}
	return items, nil
}

func (r *SensitiveEntryRepo) Update(ctx context.Context, entry *domain.Entry) error {
	return r.db.WithContext(ctx).Model(&SensitiveEntryModel{}).Where("id = ?", entry.ID).Updates(map[string]any{
		"dict_id": entry.DictID, "word": entry.Word, "enabled": entry.Enabled, "updated_at": time.Now(),
	}).Error
}

func (r *SensitiveEntryRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&SensitiveEntryModel{}, id).Error
}
