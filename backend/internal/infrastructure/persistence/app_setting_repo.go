package persistence

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AppSettingModel persists generic application configuration as JSON values keyed by string.
type AppSettingModel struct {
	Key       string    `gorm:"primaryKey;column:key;type:varchar(64)"`
	Value     string    `gorm:"column:value;type:json;not null"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (AppSettingModel) TableName() string { return "app_settings" }

// AppSettingRepo provides CRUD operations for app_settings.
type AppSettingRepo struct {
	db *gorm.DB
}

func NewAppSettingRepo(db *gorm.DB) *AppSettingRepo {
	return &AppSettingRepo{db: db}
}

// Get returns the raw JSON value for the given key, or empty string if missing.
func (r *AppSettingRepo) Get(ctx context.Context, key string) (string, error) {
	var model AppSettingModel
	if err := r.db.WithContext(ctx).Where("`key` = ?", key).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	return model.Value, nil
}

// Set upserts the JSON value for the given key.
func (r *AppSettingRepo) Set(ctx context.Context, key, value string) error {
	now := time.Now()
	model := &AppSettingModel{Key: key, Value: value, CreatedAt: now, UpdatedAt: now}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"value":      value,
			"updated_at": now,
		}),
	}).Create(model).Error
}
