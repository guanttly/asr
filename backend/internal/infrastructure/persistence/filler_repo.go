package persistence

import (
	"context"
	"time"

	domain "github.com/lgt/asr/internal/domain/filler"
	"gorm.io/gorm"
)

type FillerDictModel struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	Name        string `gorm:"type:varchar(128);not null"`
	Scene       string `gorm:"type:varchar(128);index;not null"`
	Description string `gorm:"type:text"`
	IsBase      bool   `gorm:"not null;default:false;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (FillerDictModel) TableName() string { return "filler_dicts" }

type FillerEntryModel struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	DictID    uint64 `gorm:"index;not null"`
	Word      string `gorm:"type:varchar(255);not null"`
	Enabled   bool   `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (FillerEntryModel) TableName() string { return "filler_entries" }

type FillerDictRepo struct{ db *gorm.DB }

func NewFillerDictRepo(db *gorm.DB) *FillerDictRepo { return &FillerDictRepo{db: db} }

func (r *FillerDictRepo) Create(ctx context.Context, dict *domain.Dict) error {
	model := &FillerDictModel{Name: dict.Name, Scene: dict.Scene, Description: dict.Description, IsBase: dict.IsBase}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	dict.ID = model.ID
	dict.CreatedAt = model.CreatedAt
	dict.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *FillerDictRepo) GetByID(ctx context.Context, id uint64) (*domain.Dict, error) {
	var model FillerDictModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return &domain.Dict{ID: model.ID, Name: model.Name, Scene: model.Scene, Description: model.Description, IsBase: model.IsBase, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}, nil
}

func (r *FillerDictRepo) Update(ctx context.Context, dict *domain.Dict) error {
	return r.db.WithContext(ctx).Model(&FillerDictModel{}).Where("id = ?", dict.ID).Updates(map[string]any{
		"name": dict.Name, "scene": dict.Scene, "description": dict.Description, "is_base": dict.IsBase, "updated_at": time.Now(),
	}).Error
}

func (r *FillerDictRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&FillerDictModel{}, id).Error
}

func (r *FillerDictRepo) List(ctx context.Context, offset, limit int) ([]*domain.Dict, int64, error) {
	var models []FillerDictModel
	var total int64
	query := r.db.WithContext(ctx).Model(&FillerDictModel{})
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

type FillerEntryRepo struct{ db *gorm.DB }

func NewFillerEntryRepo(db *gorm.DB) *FillerEntryRepo { return &FillerEntryRepo{db: db} }

func (r *FillerEntryRepo) Create(ctx context.Context, entry *domain.Entry) error {
	model := &FillerEntryModel{DictID: entry.DictID, Word: entry.Word, Enabled: entry.Enabled}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	entry.ID = model.ID
	entry.CreatedAt = model.CreatedAt
	entry.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *FillerEntryRepo) GetByID(ctx context.Context, id uint64) (*domain.Entry, error) {
	var model FillerEntryModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return &domain.Entry{ID: model.ID, DictID: model.DictID, Word: model.Word, Enabled: model.Enabled, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}, nil
}

func (r *FillerEntryRepo) ListByDict(ctx context.Context, dictID uint64) ([]domain.Entry, error) {
	var models []FillerEntryModel
	if err := r.db.WithContext(ctx).Where("dict_id = ?", dictID).Order("id asc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]domain.Entry, len(models))
	for i, model := range models {
		items[i] = domain.Entry{ID: model.ID, DictID: model.DictID, Word: model.Word, Enabled: model.Enabled, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
	}
	return items, nil
}

func (r *FillerEntryRepo) ListAppliedByDict(ctx context.Context, dictID uint64) ([]domain.Entry, error) {
	query := r.db.WithContext(ctx).
		Table("filler_entries AS e").
		Select("e.id, e.dict_id, e.word, e.enabled, e.created_at, e.updated_at").
		Joins("JOIN filler_dicts AS d ON d.id = e.dict_id").
		Where("e.enabled = ?", true)
	if dictID > 0 {
		query = query.Where("d.is_base = ? OR d.id = ?", true, dictID)
	} else {
		query = query.Where("d.is_base = ?", true)
	}
	var models []FillerEntryModel
	if err := query.Order("CHAR_LENGTH(e.word) desc, e.id asc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]domain.Entry, len(models))
	for i, model := range models {
		items[i] = domain.Entry{ID: model.ID, DictID: model.DictID, Word: model.Word, Enabled: model.Enabled, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
	}
	return items, nil
}

func (r *FillerEntryRepo) Update(ctx context.Context, entry *domain.Entry) error {
	return r.db.WithContext(ctx).Model(&FillerEntryModel{}).Where("id = ?", entry.ID).Updates(map[string]any{
		"dict_id": entry.DictID, "word": entry.Word, "enabled": entry.Enabled, "updated_at": time.Now(),
	}).Error
}

func (r *FillerEntryRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&FillerEntryModel{}, id).Error
}
