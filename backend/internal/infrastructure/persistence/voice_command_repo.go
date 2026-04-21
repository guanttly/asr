package persistence

import (
	"context"
	"encoding/json"
	"time"

	domain "github.com/lgt/asr/internal/domain/voicecommand"
	"gorm.io/gorm"
)

type VoiceCommandDictModel struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	Name        string `gorm:"type:varchar(128);not null"`
	GroupKey    string `gorm:"type:varchar(64);uniqueIndex;not null"`
	Description string `gorm:"type:text"`
	IsBase      bool   `gorm:"not null;default:false;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (VoiceCommandDictModel) TableName() string { return "voice_command_dicts" }

type VoiceCommandEntryModel struct {
	ID             uint64 `gorm:"primaryKey;autoIncrement"`
	DictID         uint64 `gorm:"index;not null"`
	Intent         string `gorm:"type:varchar(128);not null"`
	Label          string `gorm:"type:varchar(128);not null"`
	UtterancesJSON string `gorm:"column:utterances_json;type:json"`
	Enabled        bool   `gorm:"not null;default:true"`
	SortOrder      int    `gorm:"not null;default:0"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (VoiceCommandEntryModel) TableName() string { return "voice_command_entries" }

type VoiceCommandDictRepo struct{ db *gorm.DB }

func NewVoiceCommandDictRepo(db *gorm.DB) *VoiceCommandDictRepo { return &VoiceCommandDictRepo{db: db} }

func (r *VoiceCommandDictRepo) Create(ctx context.Context, dict *domain.Dict) error {
	model := &VoiceCommandDictModel{Name: dict.Name, GroupKey: dict.GroupKey, Description: dict.Description, IsBase: dict.IsBase}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	dict.ID = model.ID
	dict.CreatedAt = model.CreatedAt
	dict.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *VoiceCommandDictRepo) GetByID(ctx context.Context, id uint64) (*domain.Dict, error) {
	var model VoiceCommandDictModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return &domain.Dict{ID: model.ID, Name: model.Name, GroupKey: model.GroupKey, Description: model.Description, IsBase: model.IsBase, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}, nil
}

func (r *VoiceCommandDictRepo) Update(ctx context.Context, dict *domain.Dict) error {
	return r.db.WithContext(ctx).Model(&VoiceCommandDictModel{}).Where("id = ?", dict.ID).Updates(map[string]any{
		"name": dict.Name, "group_key": dict.GroupKey, "description": dict.Description, "is_base": dict.IsBase, "updated_at": time.Now(),
	}).Error
}

func (r *VoiceCommandDictRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&VoiceCommandDictModel{}, id).Error
}

func (r *VoiceCommandDictRepo) List(ctx context.Context, offset, limit int) ([]*domain.Dict, int64, error) {
	var models []VoiceCommandDictModel
	var total int64
	query := r.db.WithContext(ctx).Model(&VoiceCommandDictModel{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("is_base desc, created_at desc").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}
	items := make([]*domain.Dict, len(models))
	for i, model := range models {
		items[i] = &domain.Dict{ID: model.ID, Name: model.Name, GroupKey: model.GroupKey, Description: model.Description, IsBase: model.IsBase, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
	}
	return items, total, nil
}

func (r *VoiceCommandDictRepo) ListByIDs(ctx context.Context, ids []uint64) ([]*domain.Dict, error) {
	if len(ids) == 0 {
		return []*domain.Dict{}, nil
	}
	var models []VoiceCommandDictModel
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Order("is_base desc, created_at asc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*domain.Dict, len(models))
	for i, model := range models {
		items[i] = &domain.Dict{ID: model.ID, Name: model.Name, GroupKey: model.GroupKey, Description: model.Description, IsBase: model.IsBase, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
	}
	return items, nil
}

type VoiceCommandEntryRepo struct{ db *gorm.DB }

func NewVoiceCommandEntryRepo(db *gorm.DB) *VoiceCommandEntryRepo {
	return &VoiceCommandEntryRepo{db: db}
}

func (r *VoiceCommandEntryRepo) Create(ctx context.Context, entry *domain.Entry) error {
	payload, err := json.Marshal(entry.Utterances)
	if err != nil {
		return err
	}
	model := &VoiceCommandEntryModel{DictID: entry.DictID, Intent: entry.Intent, Label: entry.Label, UtterancesJSON: string(payload), Enabled: entry.Enabled, SortOrder: entry.SortOrder}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	entry.ID = model.ID
	entry.CreatedAt = model.CreatedAt
	entry.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *VoiceCommandEntryRepo) GetByID(ctx context.Context, id uint64) (*domain.Entry, error) {
	var model VoiceCommandEntryModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return toVoiceCommandEntry(&model), nil
}

func (r *VoiceCommandEntryRepo) ListByDict(ctx context.Context, dictID uint64) ([]domain.Entry, error) {
	var models []VoiceCommandEntryModel
	if err := r.db.WithContext(ctx).Where("dict_id = ?", dictID).Order("sort_order asc, id asc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]domain.Entry, len(models))
	for i := range models {
		items[i] = *toVoiceCommandEntry(&models[i])
	}
	return items, nil
}

func (r *VoiceCommandEntryRepo) ListByDicts(ctx context.Context, dictIDs []uint64) ([]domain.Entry, error) {
	if len(dictIDs) == 0 {
		return []domain.Entry{}, nil
	}
	var models []VoiceCommandEntryModel
	if err := r.db.WithContext(ctx).Where("dict_id IN ?", dictIDs).Order("sort_order asc, id asc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]domain.Entry, len(models))
	for i := range models {
		items[i] = *toVoiceCommandEntry(&models[i])
	}
	return items, nil
}

func (r *VoiceCommandEntryRepo) Update(ctx context.Context, entry *domain.Entry) error {
	payload, err := json.Marshal(entry.Utterances)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&VoiceCommandEntryModel{}).Where("id = ?", entry.ID).Updates(map[string]any{
		"dict_id": entry.DictID, "intent": entry.Intent, "label": entry.Label, "utterances_json": string(payload), "enabled": entry.Enabled, "sort_order": entry.SortOrder, "updated_at": time.Now(),
	}).Error
}

func (r *VoiceCommandEntryRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&VoiceCommandEntryModel{}, id).Error
}

func toVoiceCommandEntry(model *VoiceCommandEntryModel) *domain.Entry {
	utterances := []string{}
	if model.UtterancesJSON != "" {
		_ = json.Unmarshal([]byte(model.UtterancesJSON), &utterances)
	}
	return &domain.Entry{ID: model.ID, DictID: model.DictID, Intent: model.Intent, Label: model.Label, Utterances: utterances, Enabled: model.Enabled, SortOrder: model.SortOrder, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
}
