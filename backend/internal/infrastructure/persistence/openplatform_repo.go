package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	openplatform "github.com/lgt/asr/internal/domain/openplatform"
	"gorm.io/gorm"
)

type OpenAppModel struct {
	ID                    uint64    `gorm:"primaryKey;autoIncrement"`
	AppID                 string    `gorm:"column:app_id;type:varchar(48);uniqueIndex;not null"`
	Name                  string    `gorm:"type:varchar(64);uniqueIndex;not null"`
	Description           string    `gorm:"type:varchar(512)"`
	SecretHint            string    `gorm:"column:secret_hint;type:varchar(32);not null;default:''"`
	AppSecretHash         string    `gorm:"column:app_secret_hash;type:varchar(255);not null"`
	AppSecretCiphertext   string    `gorm:"column:app_secret_ciphertext;type:text"`
	SecretVersion         uint32    `gorm:"column:secret_version;not null;default:1"`
	Status                string    `gorm:"type:enum('active','disabled','revoked');not null;default:'active'"`
	OwnerUserID           uint64    `gorm:"column:owner_user_id;index;not null"`
	RateLimitPerSec       uint32    `gorm:"column:rate_limit_per_sec;not null;default:30"`
	CallbackWhitelistJSON string    `gorm:"column:callback_whitelist;type:json"`
	DefaultWorkflowsJSON  string    `gorm:"column:default_workflows;type:json"`
	MetaJSON              string    `gorm:"column:meta;type:json"`
	CreatedAt             time.Time `gorm:"not null"`
	UpdatedAt             time.Time `gorm:"not null"`
}

func (OpenAppModel) TableName() string { return "open_apps" }

type OpenAppCapabilityModel struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	OpenAppID  uint64    `gorm:"column:app_id;uniqueIndex:uk_app_cap,priority:1;index;not null"`
	Capability string    `gorm:"type:varchar(64);uniqueIndex:uk_app_cap,priority:2;not null"`
	Enabled    bool      `gorm:"not null;default:true"`
	CreatedAt  time.Time `gorm:"not null"`
	UpdatedAt  time.Time `gorm:"not null"`
}

func (OpenAppCapabilityModel) TableName() string { return "open_app_capabilities" }

type OpenSkillModel struct {
	ID                  uint64     `gorm:"primaryKey;autoIncrement"`
	SkillUID            string     `gorm:"column:skill_uid;type:varchar(48);uniqueIndex;not null"`
	AppID               uint64     `gorm:"column:app_id;index;not null"`
	Name                string     `gorm:"type:varchar(64);uniqueIndex:uk_app_name,priority:2;not null"`
	DisplayName         string     `gorm:"column:display_name;type:varchar(128);not null"`
	Description         string     `gorm:"type:varchar(512)"`
	IntentPatternsJSON  string     `gorm:"column:intent_patterns;type:json;not null"`
	ParametersSchema    string     `gorm:"column:parameters_schema;type:json"`
	CallbackURL         string     `gorm:"column:callback_url;type:varchar(512);not null"`
	CallbackTimeoutMs   uint32     `gorm:"column:callback_timeout_ms;not null;default:3000"`
	Enabled             bool       `gorm:"not null;default:true"`
	ConsecutiveFailures uint32     `gorm:"column:consecutive_failures;not null;default:0"`
	LastFailureAt       *time.Time `gorm:"column:last_failure_at"`
	CreatedAt           time.Time  `gorm:"not null"`
	UpdatedAt           time.Time  `gorm:"not null"`
}

func (OpenSkillModel) TableName() string { return "open_skills" }

type OpenCallLogModel struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	RequestID  string    `gorm:"column:request_id;type:varchar(48);index;not null"`
	AppID      uint64    `gorm:"column:app_id;index:idx_app_time,priority:1;not null"`
	Capability string    `gorm:"type:varchar(64);index:idx_capability_time,priority:1;not null"`
	Route      string    `gorm:"type:varchar(255);not null"`
	HTTPStatus uint16    `gorm:"column:http_status;not null"`
	ErrCode    string    `gorm:"column:err_code;type:varchar(64)"`
	LatencyMs  uint32    `gorm:"column:latency_ms;not null"`
	IP         string    `gorm:"column:ip;type:varchar(64)"`
	UserAgent  string    `gorm:"column:user_agent;type:varchar(255)"`
	BodyRef    string    `gorm:"column:body_ref;type:varchar(255)"`
	CreatedAt  time.Time `gorm:"index:idx_app_time,priority:2;index:idx_capability_time,priority:2;not null"`
}

func (OpenCallLogModel) TableName() string { return "open_call_logs" }

type SkillInvocationModel struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	SkillID        uint64    `gorm:"column:skill_id;index:idx_skill_time,priority:1;not null"`
	AppID          uint64    `gorm:"column:app_id;index:idx_invocation_app_time,priority:1;not null"`
	RequestID      string    `gorm:"column:request_id;type:varchar(48);not null"`
	MatchedPattern string    `gorm:"column:matched_pattern;type:varchar(255)"`
	Utterance      string    `gorm:"type:text"`
	ParametersJSON string    `gorm:"column:parameters;type:json"`
	Status         string    `gorm:"type:enum('success','failed','timeout','signed_rejected');not null"`
	HTTPStatus     *uint16   `gorm:"column:http_status"`
	LatencyMs      *uint32   `gorm:"column:latency_ms"`
	ErrorMessage   string    `gorm:"column:error_message;type:varchar(512)"`
	CreatedAt      time.Time `gorm:"index:idx_skill_time,priority:2;index:idx_invocation_app_time,priority:2;not null"`
}

func (SkillInvocationModel) TableName() string { return "skill_invocations" }

type OpenAppRepo struct {
	db *gorm.DB
}

func NewOpenAppRepo(db *gorm.DB) *OpenAppRepo {
	return &OpenAppRepo{db: db}
}

func (r *OpenAppRepo) Create(ctx context.Context, app *openplatform.App) error {
	model := &OpenAppModel{
		AppID:                 app.AppID,
		Name:                  app.Name,
		Description:           app.Description,
		SecretHint:            app.SecretHint,
		AppSecretHash:         app.AppSecretHash,
		AppSecretCiphertext:   app.AppSecretCiphertext,
		SecretVersion:         valueOrDefaultUint32(app.SecretVersion, 1),
		Status:                string(defaultAppStatus(app.Status)),
		OwnerUserID:           app.OwnerUserID,
		RateLimitPerSec:       valueOrDefaultUint32(app.RateLimitPerSec, 30),
		CallbackWhitelistJSON: mustMarshalJSON(app.CallbackWhitelist, "null"),
		DefaultWorkflowsJSON:  mustMarshalJSON(app.DefaultWorkflows, "null"),
		MetaJSON:              defaultJSONString(app.MetaJSON, "null"),
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(model).Error; err != nil {
			return normalizeOpenAppError(err)
		}
		if err := replaceCapabilities(ctx, tx, model.ID, app.AllowedCaps); err != nil {
			return err
		}
		app.ID = model.ID
		app.CreatedAt = model.CreatedAt
		app.UpdatedAt = model.UpdatedAt
		return nil
	})
}

func (r *OpenAppRepo) GetByID(ctx context.Context, id uint64) (*openplatform.App, error) {
	return r.getOne(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("id = ?", id)
	})
}

func (r *OpenAppRepo) GetByAppID(ctx context.Context, appID string) (*openplatform.App, error) {
	return r.getOne(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("app_id = ?", appID)
	})
}

func (r *OpenAppRepo) GetByName(ctx context.Context, name string) (*openplatform.App, error) {
	return r.getOne(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("name = ?", name)
	})
}

func (r *OpenAppRepo) Update(ctx context.Context, app *openplatform.App) error {
	updates := map[string]any{
		"name":                  app.Name,
		"description":           app.Description,
		"secret_hint":           app.SecretHint,
		"app_secret_hash":       app.AppSecretHash,
		"app_secret_ciphertext": app.AppSecretCiphertext,
		"secret_version":        valueOrDefaultUint32(app.SecretVersion, 1),
		"status":                string(defaultAppStatus(app.Status)),
		"rate_limit_per_sec":    valueOrDefaultUint32(app.RateLimitPerSec, 30),
		"callback_whitelist":    mustMarshalJSON(app.CallbackWhitelist, "null"),
		"default_workflows":     mustMarshalJSON(app.DefaultWorkflows, "null"),
		"meta":                  defaultJSONString(app.MetaJSON, "null"),
		"updated_at":            time.Now(),
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&OpenAppModel{}).Where("id = ?", app.ID).Updates(updates).Error; err != nil {
			return normalizeOpenAppError(err)
		}
		return replaceCapabilities(ctx, tx, app.ID, app.AllowedCaps)
	})
}

func (r *OpenAppRepo) UpdateStatus(ctx context.Context, id uint64, status openplatform.AppStatus) error {
	result := r.db.WithContext(ctx).Model(&OpenAppModel{}).Where("id = ?", id).Updates(map[string]any{
		"status":     string(defaultAppStatus(status)),
		"updated_at": time.Now(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return openplatform.ErrAppNotFound
	}
	return nil
}

func (r *OpenAppRepo) List(ctx context.Context, offset, limit int) ([]*openplatform.App, int64, error) {
	var models []OpenAppModel
	var total int64

	q := r.db.WithContext(ctx).Model(&OpenAppModel{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q = q.Order("created_at DESC")
	if offset > 0 {
		q = q.Offset(offset)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, 0, err
	}

	items := make([]*openplatform.App, len(models))
	for i := range models {
		app, err := r.toDomain(ctx, &models[i])
		if err != nil {
			return nil, 0, err
		}
		items[i] = app
	}
	return items, total, nil
}

func (r *OpenAppRepo) getOne(ctx context.Context, scope func(*gorm.DB) *gorm.DB) (*openplatform.App, error) {
	var model OpenAppModel
	query := scope(r.db.WithContext(ctx).Model(&OpenAppModel{}))
	if err := query.First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, openplatform.ErrAppNotFound
		}
		return nil, err
	}
	return r.toDomain(ctx, &model)
}

func (r *OpenAppRepo) toDomain(ctx context.Context, model *OpenAppModel) (*openplatform.App, error) {
	capabilities, err := listCapabilities(ctx, r.db, model.ID)
	if err != nil {
		return nil, err
	}
	return &openplatform.App{
		ID:                  model.ID,
		AppID:               model.AppID,
		Name:                model.Name,
		Description:         model.Description,
		SecretHint:          model.SecretHint,
		AppSecretHash:       model.AppSecretHash,
		AppSecretCiphertext: model.AppSecretCiphertext,
		SecretVersion:       model.SecretVersion,
		Status:              openplatform.AppStatus(model.Status),
		OwnerUserID:         model.OwnerUserID,
		RateLimitPerSec:     model.RateLimitPerSec,
		CallbackWhitelist:   openPlatformUnmarshalStringSlice(model.CallbackWhitelistJSON),
		DefaultWorkflows:    unmarshalUint64Map(model.DefaultWorkflowsJSON),
		AllowedCaps:         capabilities,
		MetaJSON:            defaultJSONString(model.MetaJSON, "null"),
		CreatedAt:           model.CreatedAt,
		UpdatedAt:           model.UpdatedAt,
	}, nil
}

type OpenSkillRepo struct {
	db *gorm.DB
}

func NewOpenSkillRepo(db *gorm.DB) *OpenSkillRepo {
	return &OpenSkillRepo{db: db}
}

func (r *OpenSkillRepo) Create(ctx context.Context, skill *openplatform.Skill) error {
	model := &OpenSkillModel{
		SkillUID:            skill.SkillUID,
		AppID:               skill.AppID,
		Name:                skill.Name,
		DisplayName:         skill.DisplayName,
		Description:         skill.Description,
		IntentPatternsJSON:  mustMarshalJSON(skill.IntentPatterns, "[]"),
		ParametersSchema:    defaultJSONString(skill.ParametersSchema, "null"),
		CallbackURL:         skill.CallbackURL,
		CallbackTimeoutMs:   valueOrDefaultUint32(skill.CallbackTimeoutMs, 3000),
		Enabled:             skill.Enabled,
		ConsecutiveFailures: skill.ConsecutiveFailures,
		LastFailureAt:       skill.LastFailureAt,
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return normalizeOpenSkillError(err)
	}
	skill.ID = model.ID
	skill.CreatedAt = model.CreatedAt
	skill.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *OpenSkillRepo) GetByID(ctx context.Context, id uint64) (*openplatform.Skill, error) {
	return r.getOne(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("id = ?", id)
	})
}

func (r *OpenSkillRepo) GetByUID(ctx context.Context, uid string) (*openplatform.Skill, error) {
	return r.getOne(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("skill_uid = ?", uid)
	})
}

func (r *OpenSkillRepo) GetByAppAndName(ctx context.Context, appID uint64, name string) (*openplatform.Skill, error) {
	return r.getOne(ctx, func(db *gorm.DB) *gorm.DB {
		return db.Where("app_id = ? AND name = ?", appID, name)
	})
}

func (r *OpenSkillRepo) Update(ctx context.Context, skill *openplatform.Skill) error {
	result := r.db.WithContext(ctx).Model(&OpenSkillModel{}).Where("id = ?", skill.ID).Updates(map[string]any{
		"name":                 skill.Name,
		"display_name":         skill.DisplayName,
		"description":          skill.Description,
		"intent_patterns":      mustMarshalJSON(skill.IntentPatterns, "[]"),
		"parameters_schema":    defaultJSONString(skill.ParametersSchema, "null"),
		"callback_url":         skill.CallbackURL,
		"callback_timeout_ms":  valueOrDefaultUint32(skill.CallbackTimeoutMs, 3000),
		"enabled":              skill.Enabled,
		"consecutive_failures": skill.ConsecutiveFailures,
		"last_failure_at":      skill.LastFailureAt,
		"updated_at":           time.Now(),
	}).Error
	if result != nil {
		return normalizeOpenSkillError(result)
	}
	return nil
}

func (r *OpenSkillRepo) Delete(ctx context.Context, id uint64) error {
	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&OpenSkillModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return openplatform.ErrSkillNotFound
	}
	return nil
}

func (r *OpenSkillRepo) ListByApp(ctx context.Context, appID uint64) ([]*openplatform.Skill, error) {
	var models []OpenSkillModel
	if err := r.db.WithContext(ctx).Where("app_id = ?", appID).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*openplatform.Skill, len(models))
	for i := range models {
		items[i] = toOpenSkillDomain(&models[i])
	}
	return items, nil
}

func (r *OpenSkillRepo) getOne(ctx context.Context, scope func(*gorm.DB) *gorm.DB) (*openplatform.Skill, error) {
	var model OpenSkillModel
	query := scope(r.db.WithContext(ctx).Model(&OpenSkillModel{}))
	if err := query.First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, openplatform.ErrSkillNotFound
		}
		return nil, err
	}
	return toOpenSkillDomain(&model), nil
}

func toOpenSkillDomain(model *OpenSkillModel) *openplatform.Skill {
	return &openplatform.Skill{
		ID:                  model.ID,
		SkillUID:            model.SkillUID,
		AppID:               model.AppID,
		Name:                model.Name,
		DisplayName:         model.DisplayName,
		Description:         model.Description,
		IntentPatterns:      openPlatformUnmarshalStringSlice(model.IntentPatternsJSON),
		ParametersSchema:    defaultJSONString(model.ParametersSchema, "null"),
		CallbackURL:         model.CallbackURL,
		CallbackTimeoutMs:   model.CallbackTimeoutMs,
		Enabled:             model.Enabled,
		ConsecutiveFailures: model.ConsecutiveFailures,
		LastFailureAt:       model.LastFailureAt,
		CreatedAt:           model.CreatedAt,
		UpdatedAt:           model.UpdatedAt,
	}
}

type OpenCallLogRepo struct {
	db *gorm.DB
}

func NewOpenCallLogRepo(db *gorm.DB) *OpenCallLogRepo {
	return &OpenCallLogRepo{db: db}
}

func (r *OpenCallLogRepo) Create(ctx context.Context, call *openplatform.CallLog) error {
	model := &OpenCallLogModel{
		RequestID:  call.RequestID,
		AppID:      call.AppID,
		Capability: call.Capability,
		Route:      call.Route,
		HTTPStatus: call.HTTPStatus,
		ErrCode:    call.ErrCode,
		LatencyMs:  call.LatencyMs,
		IP:         call.IP,
		UserAgent:  call.UserAgent,
		BodyRef:    call.BodyRef,
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	call.ID = model.ID
	call.CreatedAt = model.CreatedAt
	return nil
}

func (r *OpenCallLogRepo) ListByApp(ctx context.Context, appID uint64, limit int) ([]*openplatform.CallLog, error) {
	var models []OpenCallLogModel
	q := r.db.WithContext(ctx).Where("app_id = ?", appID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*openplatform.CallLog, len(models))
	for i := range models {
		items[i] = &openplatform.CallLog{
			ID:         models[i].ID,
			RequestID:  models[i].RequestID,
			AppID:      models[i].AppID,
			Capability: models[i].Capability,
			Route:      models[i].Route,
			HTTPStatus: models[i].HTTPStatus,
			ErrCode:    models[i].ErrCode,
			LatencyMs:  models[i].LatencyMs,
			IP:         models[i].IP,
			UserAgent:  models[i].UserAgent,
			BodyRef:    models[i].BodyRef,
			CreatedAt:  models[i].CreatedAt,
		}
	}
	return items, nil
}

type SkillInvocationRepo struct {
	db *gorm.DB
}

func NewSkillInvocationRepo(db *gorm.DB) *SkillInvocationRepo {
	return &SkillInvocationRepo{db: db}
}

func (r *SkillInvocationRepo) Create(ctx context.Context, invocation *openplatform.SkillInvocation) error {
	model := &SkillInvocationModel{
		SkillID:        invocation.SkillID,
		AppID:          invocation.AppID,
		RequestID:      invocation.RequestID,
		MatchedPattern: invocation.MatchedPattern,
		Utterance:      invocation.Utterance,
		ParametersJSON: defaultJSONString(invocation.ParametersJSON, "null"),
		Status:         string(invocation.Status),
		HTTPStatus:     invocation.HTTPStatus,
		LatencyMs:      invocation.LatencyMs,
		ErrorMessage:   invocation.ErrorMessage,
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	invocation.ID = model.ID
	invocation.CreatedAt = model.CreatedAt
	return nil
}

func (r *SkillInvocationRepo) ListBySkill(ctx context.Context, skillID uint64, limit int) ([]*openplatform.SkillInvocation, error) {
	var models []SkillInvocationModel
	q := r.db.WithContext(ctx).Where("skill_id = ?", skillID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*openplatform.SkillInvocation, len(models))
	for i := range models {
		items[i] = &openplatform.SkillInvocation{
			ID:             models[i].ID,
			SkillID:        models[i].SkillID,
			AppID:          models[i].AppID,
			RequestID:      models[i].RequestID,
			MatchedPattern: models[i].MatchedPattern,
			Utterance:      models[i].Utterance,
			ParametersJSON: defaultJSONString(models[i].ParametersJSON, "null"),
			Status:         openplatform.InvocationStatus(models[i].Status),
			HTTPStatus:     models[i].HTTPStatus,
			LatencyMs:      models[i].LatencyMs,
			ErrorMessage:   models[i].ErrorMessage,
			CreatedAt:      models[i].CreatedAt,
		}
	}
	return items, nil
}

func replaceCapabilities(ctx context.Context, tx *gorm.DB, appID uint64, capabilities []string) error {
	if err := tx.WithContext(ctx).Where("app_id = ?", appID).Delete(&OpenAppCapabilityModel{}).Error; err != nil {
		return err
	}
	if len(capabilities) == 0 {
		return nil
	}
	now := time.Now()
	models := make([]OpenAppCapabilityModel, 0, len(capabilities))
	seen := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		normalized := strings.TrimSpace(capability)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		models = append(models, OpenAppCapabilityModel{
			OpenAppID:  appID,
			Capability: normalized,
			Enabled:    true,
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	if len(models) == 0 {
		return nil
	}
	return tx.WithContext(ctx).Create(&models).Error
}

func listCapabilities(ctx context.Context, db *gorm.DB, appID uint64) ([]string, error) {
	var models []OpenAppCapabilityModel
	if err := db.WithContext(ctx).Where("app_id = ? AND enabled = ?", appID, true).Order("capability ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]string, 0, len(models))
	for _, model := range models {
		items = append(items, model.Capability)
	}
	return items, nil
}

func defaultAppStatus(status openplatform.AppStatus) openplatform.AppStatus {
	if strings.TrimSpace(string(status)) == "" {
		return openplatform.AppStatusActive
	}
	return status
}

func valueOrDefaultUint32(value, fallback uint32) uint32 {
	if value == 0 {
		return fallback
	}
	return value
}

func defaultJSONString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func mustMarshalJSON(value any, fallback string) string {
	if value == nil {
		return fallback
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	if len(payload) == 0 {
		return fallback
	}
	return string(payload)
}

func openPlatformUnmarshalStringSlice(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var items []string
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil
	}
	return items
}

func unmarshalUint64Map(raw string) map[string]uint64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var items map[string]uint64
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil
	}
	return items
}

func normalizeOpenAppError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
		return openplatform.ErrAppAlreadyExists
	}
	return err
}

func normalizeOpenSkillError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
		return openplatform.ErrSkillNameDuplicated
	}
	return err
}
