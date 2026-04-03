package persistence

import (
	"context"
	"errors"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	domain "github.com/lgt/asr/internal/domain/user"
	"gorm.io/gorm"
)

// UserModel is the persistence model for users.
type UserModel struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	Username     string `gorm:"type:varchar(64);uniqueIndex;not null"`
	PasswordHash string `gorm:"type:varchar(255);not null"`
	DisplayName  string `gorm:"type:varchar(128)"`
	Role         string `gorm:"type:varchar(20);not null;default:'user'"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (UserModel) TableName() string { return "users" }

type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(ctx context.Context, user *domain.User) error {
	model := &UserModel{
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
		DisplayName:  user.DisplayName,
		Role:         string(user.Role),
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		var mysqlErr *mysqldriver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return domain.ErrUserAlreadyExists
		}
		return err
	}
	user.ID = model.ID
	user.CreatedAt = model.CreatedAt
	user.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *UserRepo) GetByID(ctx context.Context, id uint64) (*domain.User, error) {
	var model UserModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var model UserModel
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *UserRepo) Update(ctx context.Context, user *domain.User) error {
	return r.db.WithContext(ctx).Model(&UserModel{}).Where("id = ?", user.ID).Updates(map[string]any{
		"display_name": user.DisplayName,
		"role":         string(user.Role),
		"updated_at":   time.Now(),
	}).Error
}

func (r *UserRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&UserModel{}, id).Error
}

func (r *UserRepo) List(ctx context.Context, offset, limit int) ([]*domain.User, int64, error) {
	var models []UserModel
	var total int64
	query := r.db.WithContext(ctx).Model(&UserModel{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("created_at desc").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}
	items := make([]*domain.User, len(models))
	for i := range models {
		items[i] = r.toDomain(&models[i])
	}
	return items, total, nil
}

func (r *UserRepo) toDomain(model *UserModel) *domain.User {
	return &domain.User{
		ID:           model.ID,
		Username:     model.Username,
		PasswordHash: model.PasswordHash,
		DisplayName:  model.DisplayName,
		Role:         domain.Role(model.Role),
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}
