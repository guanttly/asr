package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	domain "github.com/lgt/asr/internal/domain/user"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserModel is the persistence model for users.
type UserModel struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	Username     string `gorm:"type:varchar(128);uniqueIndex;not null"`
	PasswordHash string `gorm:"type:varchar(255);not null"`
	DisplayName  string `gorm:"type:varchar(128)"`
	Role         string `gorm:"type:varchar(20);not null;default:'user'"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (UserModel) TableName() string { return "users" }

// DeviceIdentityModel stores the machine fingerprint for anonymous desktop login.
type DeviceIdentityModel struct {
	ID               uint64 `gorm:"primaryKey;autoIncrement"`
	UserID           uint64 `gorm:"uniqueIndex;not null"`
	MachineCode      string `gorm:"type:varchar(128);uniqueIndex;not null"`
	Hostname         string `gorm:"type:varchar(255)"`
	Platform         string `gorm:"type:varchar(64)"`
	IPAddressesJSON  string `gorm:"type:text"`
	MACAddressesJSON string `gorm:"type:text"`
	LastSeenAt       time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (DeviceIdentityModel) TableName() string { return "device_identities" }

// UserWorkflowBindingsModel stores default app workflow bindings for each user.
type UserWorkflowBindingsModel struct {
	UserID             uint64 `gorm:"primaryKey"`
	RealtimeWorkflowID *uint64
	BatchWorkflowID    *uint64
	MeetingWorkflowID  *uint64
	VoiceWorkflowID    *uint64 `gorm:"column:voice_control_workflow_id"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (UserWorkflowBindingsModel) TableName() string { return "user_workflow_bindings" }

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

func (r *UserRepo) GetDeviceIdentityByMachineCode(ctx context.Context, machineCode string) (*domain.DeviceIdentity, error) {
	var model DeviceIdentityModel
	if err := r.db.WithContext(ctx).Where("machine_code = ?", machineCode).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrDeviceIdentityNotFound
		}
		return nil, err
	}
	return toDeviceIdentity(&model), nil
}

func (r *UserRepo) GetWorkflowBindings(ctx context.Context, userID uint64) (*domain.WorkflowBindings, error) {
	var model UserWorkflowBindingsModel
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &domain.WorkflowBindings{UserID: userID}, nil
		}
		return nil, err
	}

	return &domain.WorkflowBindings{
		UserID:             model.UserID,
		RealtimeWorkflowID: model.RealtimeWorkflowID,
		BatchWorkflowID:    model.BatchWorkflowID,
		MeetingWorkflowID:  model.MeetingWorkflowID,
		VoiceWorkflowID:    model.VoiceWorkflowID,
		CreatedAt:          model.CreatedAt,
		UpdatedAt:          model.UpdatedAt,
	}, nil
}

func (r *UserRepo) UpsertDeviceIdentity(ctx context.Context, identity *domain.DeviceIdentity) error {
	model := &DeviceIdentityModel{
		UserID:           identity.UserID,
		MachineCode:      identity.MachineCode,
		Hostname:         identity.Hostname,
		Platform:         identity.Platform,
		IPAddressesJSON:  marshalStringSlice(identity.IPAddresses),
		MACAddressesJSON: marshalStringSlice(identity.MACAddresses),
		LastSeenAt:       time.Now(),
	}

	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "machine_code"}},
		DoUpdates: clause.Assignments(map[string]any{
			"user_id":            model.UserID,
			"hostname":           model.Hostname,
			"platform":           model.Platform,
			"ip_addresses_json":  model.IPAddressesJSON,
			"mac_addresses_json": model.MACAddressesJSON,
			"last_seen_at":       model.LastSeenAt,
			"updated_at":         time.Now(),
		}),
	}).Create(model).Error; err != nil {
		return err
	}

	identity.ID = model.ID
	identity.CreatedAt = model.CreatedAt
	identity.UpdatedAt = model.UpdatedAt
	identity.LastSeenAt = model.LastSeenAt
	return nil
}

func (r *UserRepo) SaveWorkflowBindings(ctx context.Context, bindings *domain.WorkflowBindings) error {
	model := &UserWorkflowBindingsModel{
		UserID:             bindings.UserID,
		RealtimeWorkflowID: bindings.RealtimeWorkflowID,
		BatchWorkflowID:    bindings.BatchWorkflowID,
		MeetingWorkflowID:  bindings.MeetingWorkflowID,
		VoiceWorkflowID:    bindings.VoiceWorkflowID,
	}

	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"realtime_workflow_id", "batch_workflow_id", "meeting_workflow_id", "voice_control_workflow_id", "updated_at"}),
	}).Create(model).Error; err != nil {
		return err
	}

	bindings.CreatedAt = model.CreatedAt
	bindings.UpdatedAt = model.UpdatedAt
	return nil
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

func toDeviceIdentity(model *DeviceIdentityModel) *domain.DeviceIdentity {
	return &domain.DeviceIdentity{
		ID:           model.ID,
		UserID:       model.UserID,
		MachineCode:  model.MachineCode,
		Hostname:     model.Hostname,
		Platform:     model.Platform,
		IPAddresses:  unmarshalStringSlice(model.IPAddressesJSON),
		MACAddresses: unmarshalStringSlice(model.MACAddressesJSON),
		LastSeenAt:   model.LastSeenAt,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}

func marshalStringSlice(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func unmarshalStringSlice(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{}
	}
	return items
}
