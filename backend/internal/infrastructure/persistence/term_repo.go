package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	domain "github.com/lgt/asr/internal/domain/terminology"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DictModel is the persistence model for terminology dictionaries.
type DictModel struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	Name      string `gorm:"type:varchar(128);not null"`
	Domain    string `gorm:"type:varchar(128);index;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (DictModel) TableName() string { return "term_dicts" }

// EntryModel is the persistence model for term entries.
type EntryModel struct {
	ID                uint64 `gorm:"primaryKey;autoIncrement"`
	DictID            uint64 `gorm:"index;not null"`
	CorrectTerm       string `gorm:"type:varchar(255);not null"`
	WrongVariantsJSON string `gorm:"column:wrong_variants_json;type:json"`
	Pinyin            string `gorm:"type:varchar(255)"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (EntryModel) TableName() string { return "term_entries" }

// RuleModel is the persistence model for correction rules.
type RuleModel struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	DictID      uint64 `gorm:"index;not null"`
	Layer       int    `gorm:"not null"`
	Pattern     string `gorm:"type:varchar(255);not null"`
	Replacement string `gorm:"type:varchar(255);not null"`
	Enabled     bool   `gorm:"not null;default:true"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (RuleModel) TableName() string { return "correction_rules" }

type DictRepo struct {
	db *gorm.DB
}

func NewDictRepo(db *gorm.DB) *DictRepo {
	return &DictRepo{db: db}
}

func (r *DictRepo) Create(ctx context.Context, dict *domain.TermDict) error {
	model := &DictModel{Name: dict.Name, Domain: dict.Domain}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	dict.ID = model.ID
	dict.CreatedAt = model.CreatedAt
	dict.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *DictRepo) GetByID(ctx context.Context, id uint64) (*domain.TermDict, error) {
	var model DictModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return &domain.TermDict{
		ID:        model.ID,
		Name:      model.Name,
		Domain:    model.Domain,
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}, nil
}

func (r *DictRepo) Update(ctx context.Context, dict *domain.TermDict) error {
	return r.db.WithContext(ctx).Model(&DictModel{}).Where("id = ?", dict.ID).Updates(map[string]any{
		"name":       dict.Name,
		"domain":     dict.Domain,
		"updated_at": time.Now(),
	}).Error
}

func (r *DictRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&DictModel{}, id).Error
}

func (r *DictRepo) List(ctx context.Context, offset, limit int) ([]*domain.TermDict, int64, error) {
	var models []DictModel
	var total int64
	query := r.db.WithContext(ctx).Model(&DictModel{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("created_at desc").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}
	items := make([]*domain.TermDict, len(models))
	for i, model := range models {
		items[i] = &domain.TermDict{
			ID:        model.ID,
			Name:      model.Name,
			Domain:    model.Domain,
			CreatedAt: model.CreatedAt,
			UpdatedAt: model.UpdatedAt,
		}
	}
	return items, total, nil
}

type EntryRepo struct {
	db *gorm.DB
}

func NewEntryRepo(db *gorm.DB) *EntryRepo {
	return &EntryRepo{db: db}
}

func (r *EntryRepo) BatchCreate(ctx context.Context, entries []domain.TermEntry) error {
	models := make([]EntryModel, len(entries))
	for i, entry := range entries {
		payload, err := json.Marshal(entry.WrongVariants)
		if err != nil {
			return err
		}
		models[i] = EntryModel{
			DictID:            entry.DictID,
			CorrectTerm:       entry.CorrectTerm,
			WrongVariantsJSON: string(payload),
			Pinyin:            entry.Pinyin,
		}
	}
	return r.db.WithContext(ctx).Create(&models).Error
}

func (r *EntryRepo) GetByID(ctx context.Context, id uint64) (*domain.TermEntry, error) {
	var model EntryModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	var variants []string
	if model.WrongVariantsJSON != "" {
		_ = json.Unmarshal([]byte(model.WrongVariantsJSON), &variants)
	}
	return &domain.TermEntry{
		ID:            model.ID,
		DictID:        model.DictID,
		CorrectTerm:   model.CorrectTerm,
		WrongVariants: variants,
		Pinyin:        model.Pinyin,
	}, nil
}

func (r *EntryRepo) ListByDict(ctx context.Context, dictID uint64) ([]domain.TermEntry, error) {
	var models []EntryModel
	if err := r.db.WithContext(ctx).Where("dict_id = ?", dictID).Order("id asc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]domain.TermEntry, len(models))
	for i, model := range models {
		var variants []string
		if model.WrongVariantsJSON != "" {
			_ = json.Unmarshal([]byte(model.WrongVariantsJSON), &variants)
		}
		items[i] = domain.TermEntry{
			ID:            model.ID,
			DictID:        model.DictID,
			CorrectTerm:   model.CorrectTerm,
			WrongVariants: variants,
			Pinyin:        model.Pinyin,
		}
	}
	return items, nil
}

func (r *EntryRepo) Update(ctx context.Context, entry *domain.TermEntry) error {
	payload, err := json.Marshal(entry.WrongVariants)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&EntryModel{}).Where("id = ?", entry.ID).Updates(map[string]any{
		"correct_term":        entry.CorrectTerm,
		"wrong_variants_json": string(payload),
		"pinyin":              entry.Pinyin,
		"updated_at":          time.Now(),
	}).Error
}

func (r *EntryRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&EntryModel{}, id).Error
}

type RuleRepo struct {
	db *gorm.DB
}

func NewRuleRepo(db *gorm.DB) *RuleRepo {
	return &RuleRepo{db: db}
}

func (r *RuleRepo) Create(ctx context.Context, rule *domain.CorrectionRule) error {
	model := &RuleModel{
		DictID:      rule.DictID,
		Layer:       int(rule.Layer),
		Pattern:     rule.Pattern,
		Replacement: rule.Replacement,
		Enabled:     rule.Enabled,
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	rule.ID = model.ID
	rule.CreatedAt = model.CreatedAt
	return nil
}

func (r *RuleRepo) GetByID(ctx context.Context, id uint64) (*domain.CorrectionRule, error) {
	var model RuleModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return &domain.CorrectionRule{
		ID:          model.ID,
		DictID:      model.DictID,
		Layer:       domain.CorrectionLayer(model.Layer),
		Pattern:     model.Pattern,
		Replacement: model.Replacement,
		Enabled:     model.Enabled,
		CreatedAt:   model.CreatedAt,
	}, nil
}

func (r *RuleRepo) ListByDict(ctx context.Context, dictID uint64) ([]domain.CorrectionRule, error) {
	var models []RuleModel
	if err := r.db.WithContext(ctx).Where("dict_id = ?", dictID).Order("layer asc, id asc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]domain.CorrectionRule, len(models))
	for i, model := range models {
		items[i] = domain.CorrectionRule{
			ID:          model.ID,
			DictID:      model.DictID,
			Layer:       domain.CorrectionLayer(model.Layer),
			Pattern:     model.Pattern,
			Replacement: model.Replacement,
			Enabled:     model.Enabled,
			CreatedAt:   model.CreatedAt,
		}
	}
	return items, nil
}

func (r *RuleRepo) Update(ctx context.Context, rule *domain.CorrectionRule) error {
	return r.db.WithContext(ctx).Model(&RuleModel{}).Where("id = ?", rule.ID).Updates(map[string]any{
		"layer":       int(rule.Layer),
		"pattern":     rule.Pattern,
		"replacement": rule.Replacement,
		"enabled":     rule.Enabled,
		"updated_at":  time.Now(),
	}).Error
}

func (r *RuleRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&RuleModel{}, id).Error
}

type SeedStateRepo struct {
	db *gorm.DB
}

func NewSeedStateRepo(db *gorm.DB) *SeedStateRepo {
	return &SeedStateRepo{db: db}
}

func (r *SeedStateRepo) IsSeeded(ctx context.Context, key string) (bool, error) {
	var model AdminOperationStateModel
	if err := r.db.WithContext(ctx).First(&model, "operation_key = ?", key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *SeedStateRepo) MarkSeeded(ctx context.Context, key string) error {
	model := &AdminOperationStateModel{
		OperationKey: key,
		Payload:      `{"seeded":true}`,
	}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "operation_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"payload", "updated_at"}),
		}).
		Create(model).Error
}
