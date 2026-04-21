package persistence

import (
	"context"
	"encoding/json"
	"time"

	domain "github.com/lgt/asr/internal/domain/workflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ─── GORM Models ────────────────────────────────────────

type WorkflowModel struct {
	ID                uint64  `gorm:"primaryKey;autoIncrement"`
	Name              string  `gorm:"type:varchar(255);not null"`
	Description       string  `gorm:"type:text"`
	WorkflowType      string  `gorm:"type:varchar(50);not null;default:'legacy';index"`
	SourceKind        string  `gorm:"type:varchar(50);not null;default:'legacy_text';index"`
	TargetKind        string  `gorm:"type:varchar(50);not null;default:'transcript';index"`
	IsLegacy          bool    `gorm:"not null;default:true;index"`
	ValidationMessage string  `gorm:"type:text"`
	OwnerType         string  `gorm:"type:enum('system','user');not null;default:'user'"`
	OwnerID           uint64  `gorm:"not null"`
	SourceID          *uint64 `gorm:"default:null"`
	IsPublished       bool    `gorm:"not null;default:false"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (WorkflowModel) TableName() string { return "workflows" }

type WorkflowNodeModel struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`
	WorkflowID uint64 `gorm:"index;not null"`
	NodeType   string `gorm:"type:varchar(50);not null"`
	Position   int    `gorm:"not null;default:0"`
	Config     string `gorm:"type:json"`
	Enabled    bool   `gorm:"not null;default:true"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (WorkflowNodeModel) TableName() string { return "workflow_nodes" }

type WorkflowNodeDefaultModel struct {
	NodeType  string `gorm:"primaryKey;type:varchar(50)"`
	Config    string `gorm:"type:json"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (WorkflowNodeDefaultModel) TableName() string { return "workflow_node_defaults" }

type WorkflowExecutionModel struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	WorkflowID   uint64 `gorm:"index;not null"`
	TriggerType  string `gorm:"type:varchar(50);not null"`
	TriggerID    string `gorm:"type:varchar(128)"`
	InputText    string `gorm:"type:longtext"`
	FinalText    string `gorm:"type:longtext"`
	Status       string `gorm:"type:varchar(20);not null;default:'pending';index"`
	ErrorMessage string `gorm:"type:text"`
	StartedAt    *time.Time
	CompletedAt  *time.Time
	CreatedAt    time.Time
}

func (WorkflowExecutionModel) TableName() string { return "workflow_executions" }

type WorkflowNodeResultModel struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	ExecutionID uint64 `gorm:"index;not null"`
	NodeID      uint64 `gorm:"not null"`
	NodeType    string `gorm:"type:varchar(50);not null"`
	Position    int    `gorm:"not null;default:0"`
	InputText   string `gorm:"type:longtext"`
	OutputText  string `gorm:"type:longtext"`
	Status      string `gorm:"type:varchar(20);not null;default:'pending'"`
	Detail      string `gorm:"type:json"`
	DurationMs  int    `gorm:"not null;default:0"`
	ExecutedAt  *time.Time
}

func (WorkflowNodeResultModel) TableName() string { return "workflow_node_results" }

// ─── WorkflowRepo ───────────────────────────────────────

type WorkflowRepo struct {
	db *gorm.DB
}

func NewWorkflowRepo(db *gorm.DB) *WorkflowRepo {
	return &WorkflowRepo{db: db}
}

func (r *WorkflowRepo) Create(ctx context.Context, wf *domain.Workflow) error {
	m := &WorkflowModel{
		Name:              wf.Name,
		Description:       wf.Description,
		WorkflowType:      string(wf.WorkflowType),
		SourceKind:        string(wf.SourceKind),
		TargetKind:        string(wf.TargetKind),
		IsLegacy:          wf.IsLegacy,
		ValidationMessage: wf.ValidationMessage,
		OwnerType:         string(wf.OwnerType),
		OwnerID:           wf.OwnerID,
		SourceID:          wf.SourceID,
		IsPublished:       wf.IsPublished,
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return err
	}
	wf.ID = m.ID
	wf.CreatedAt = m.CreatedAt
	wf.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *WorkflowRepo) GetByID(ctx context.Context, id uint64) (*domain.Workflow, error) {
	var m WorkflowModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		return nil, err
	}
	return r.toDomain(&m), nil
}

func (r *WorkflowRepo) Update(ctx context.Context, wf *domain.Workflow) error {
	return r.db.WithContext(ctx).Model(&WorkflowModel{}).Where("id = ?", wf.ID).Updates(map[string]interface{}{
		"name":               wf.Name,
		"description":        wf.Description,
		"workflow_type":      wf.WorkflowType,
		"source_kind":        wf.SourceKind,
		"target_kind":        wf.TargetKind,
		"is_legacy":          wf.IsLegacy,
		"validation_message": wf.ValidationMessage,
		"source_id":          wf.SourceID,
		"is_published":       wf.IsPublished,
		"updated_at":         time.Now(),
	}).Error
}

func (r *WorkflowRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&WorkflowModel{}).Error
}

func (r *WorkflowRepo) List(ctx context.Context, ownerType *domain.OwnerType, ownerID *uint64, publishedOnly bool, offset, limit int) ([]*domain.Workflow, int64, error) {
	return r.listWithFilter(ctx, ownerType, ownerID, publishedOnly, domain.WorkflowListFilter{IncludeLegacy: true}, offset, limit)
}

func (r *WorkflowRepo) ListFiltered(ctx context.Context, ownerType *domain.OwnerType, ownerID *uint64, publishedOnly bool, filter domain.WorkflowListFilter, offset, limit int) ([]*domain.Workflow, int64, error) {
	return r.listWithFilter(ctx, ownerType, ownerID, publishedOnly, filter, offset, limit)
}

func (r *WorkflowRepo) listWithFilter(ctx context.Context, ownerType *domain.OwnerType, ownerID *uint64, publishedOnly bool, filter domain.WorkflowListFilter, offset, limit int) ([]*domain.Workflow, int64, error) {
	var models []WorkflowModel
	var total int64

	q := r.db.WithContext(ctx).Model(&WorkflowModel{})
	if ownerType != nil {
		q = q.Where("owner_type = ?", string(*ownerType))
	}
	if ownerID != nil {
		q = q.Where("owner_id = ?", *ownerID)
	}
	if publishedOnly {
		q = q.Where("is_published = ?", true)
	}
	if !filter.IncludeLegacy {
		q = q.Where("is_legacy = ?", false)
	}
	if filter.WorkflowType != nil {
		q = q.Where("workflow_type = ?", string(*filter.WorkflowType))
	}
	if filter.SourceKind != nil {
		q = q.Where("source_kind = ?", string(*filter.SourceKind))
	}
	if filter.TargetKind != nil {
		q = q.Where("target_kind = ?", string(*filter.TargetKind))
	}

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

	items := make([]*domain.Workflow, len(models))
	for i := range models {
		items[i] = r.toDomain(&models[i])
	}
	return items, total, nil
}

func (r *WorkflowRepo) toDomain(m *WorkflowModel) *domain.Workflow {
	return &domain.Workflow{
		ID:                m.ID,
		Name:              m.Name,
		Description:       m.Description,
		WorkflowType:      domain.WorkflowType(m.WorkflowType),
		SourceKind:        domain.WorkflowSourceKind(m.SourceKind),
		TargetKind:        domain.WorkflowTargetKind(m.TargetKind),
		IsLegacy:          m.IsLegacy,
		ValidationMessage: m.ValidationMessage,
		OwnerType:         domain.OwnerType(m.OwnerType),
		OwnerID:           m.OwnerID,
		SourceID:          m.SourceID,
		IsPublished:       m.IsPublished,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

// ─── NodeRepo ───────────────────────────────────────────

type WorkflowNodeRepo struct {
	db *gorm.DB
}

func NewWorkflowNodeRepo(db *gorm.DB) *WorkflowNodeRepo {
	return &WorkflowNodeRepo{db: db}
}

func (r *WorkflowNodeRepo) ListByWorkflow(ctx context.Context, workflowID uint64) ([]domain.Node, error) {
	var models []WorkflowNodeModel
	if err := r.db.WithContext(ctx).Where("workflow_id = ?", workflowID).Order("position ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]domain.Node, len(models))
	for i, m := range models {
		items[i] = domain.Node{
			ID:         m.ID,
			WorkflowID: m.WorkflowID,
			NodeType:   domain.NodeType(m.NodeType),
			Position:   m.Position,
			Config:     m.Config,
			Enabled:    m.Enabled,
			CreatedAt:  m.CreatedAt,
			UpdatedAt:  m.UpdatedAt,
		}
	}
	return items, nil
}

func (r *WorkflowNodeRepo) BatchSave(ctx context.Context, workflowID uint64, nodes []domain.Node) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete existing nodes
		if err := tx.Where("workflow_id = ?", workflowID).Delete(&WorkflowNodeModel{}).Error; err != nil {
			return err
		}
		if len(nodes) == 0 {
			return nil
		}
		// Insert new nodes
		models := make([]WorkflowNodeModel, len(nodes))
		for i, n := range nodes {
			models[i] = WorkflowNodeModel{
				WorkflowID: workflowID,
				NodeType:   string(n.NodeType),
				Position:   n.Position,
				Config:     n.Config,
				Enabled:    n.Enabled,
			}
		}
		return tx.Create(&models).Error
	})
}

func (r *WorkflowNodeRepo) DeleteByWorkflow(ctx context.Context, workflowID uint64) error {
	return r.db.WithContext(ctx).Where("workflow_id = ?", workflowID).Delete(&WorkflowNodeModel{}).Error
}

// ─── NodeDefaultRepo ───────────────────────────────────

type WorkflowNodeDefaultRepo struct {
	db *gorm.DB
}

func NewWorkflowNodeDefaultRepo(db *gorm.DB) *WorkflowNodeDefaultRepo {
	return &WorkflowNodeDefaultRepo{db: db}
}

func (r *WorkflowNodeDefaultRepo) List(ctx context.Context) ([]domain.NodeDefault, error) {
	var models []WorkflowNodeDefaultModel
	if err := r.db.WithContext(ctx).Order("node_type asc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]domain.NodeDefault, len(models))
	for i, model := range models {
		items[i] = domain.NodeDefault{
			NodeType:  domain.NodeType(model.NodeType),
			Config:    model.Config,
			CreatedAt: model.CreatedAt,
			UpdatedAt: model.UpdatedAt,
		}
	}
	return items, nil
}

func (r *WorkflowNodeDefaultRepo) GetByType(ctx context.Context, nodeType domain.NodeType) (*domain.NodeDefault, error) {
	var model WorkflowNodeDefaultModel
	if err := r.db.WithContext(ctx).Where("node_type = ?", string(nodeType)).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &domain.NodeDefault{
		NodeType:  domain.NodeType(model.NodeType),
		Config:    model.Config,
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}, nil
}

func (r *WorkflowNodeDefaultRepo) Upsert(ctx context.Context, item *domain.NodeDefault) error {
	now := time.Now()
	model := &WorkflowNodeDefaultModel{
		NodeType:  string(item.NodeType),
		Config:    item.Config,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "node_type"}},
		DoUpdates: clause.Assignments(map[string]any{
			"config":     model.Config,
			"updated_at": now,
		}),
	}).Create(model).Error
}

// ─── ExecutionRepo ──────────────────────────────────────

type WorkflowExecutionRepo struct {
	db *gorm.DB
}

func NewWorkflowExecutionRepo(db *gorm.DB) *WorkflowExecutionRepo {
	return &WorkflowExecutionRepo{db: db}
}

func (r *WorkflowExecutionRepo) Create(ctx context.Context, exec *domain.Execution) error {
	m := &WorkflowExecutionModel{
		WorkflowID:  exec.WorkflowID,
		TriggerType: string(exec.TriggerType),
		TriggerID:   exec.TriggerID,
		InputText:   exec.InputText,
		FinalText:   exec.FinalText,
		Status:      string(exec.Status),
		StartedAt:   exec.StartedAt,
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return err
	}
	exec.ID = m.ID
	exec.CreatedAt = m.CreatedAt
	return nil
}

func (r *WorkflowExecutionRepo) GetByID(ctx context.Context, id uint64) (*domain.Execution, error) {
	var m WorkflowExecutionModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		return nil, err
	}
	return r.toDomain(&m), nil
}

func (r *WorkflowExecutionRepo) Update(ctx context.Context, exec *domain.Execution) error {
	return r.db.WithContext(ctx).Model(&WorkflowExecutionModel{}).Where("id = ?", exec.ID).Updates(map[string]interface{}{
		"final_text":    exec.FinalText,
		"status":        string(exec.Status),
		"error_message": exec.ErrorMessage,
		"completed_at":  exec.CompletedAt,
	}).Error
}

func (r *WorkflowExecutionRepo) ListByWorkflow(ctx context.Context, workflowID uint64, offset, limit int) ([]*domain.Execution, int64, error) {
	return r.listWith(ctx, "workflow_id = ?", workflowID, offset, limit)
}

func (r *WorkflowExecutionRepo) ListByTrigger(ctx context.Context, triggerType domain.TriggerType, triggerID string, offset, limit int) ([]*domain.Execution, int64, error) {
	return r.listWith(ctx, "trigger_type = ? AND trigger_id = ?", []interface{}{string(triggerType), triggerID}, offset, limit)
}

func (r *WorkflowExecutionRepo) listWith(ctx context.Context, where string, args interface{}, offset, limit int) ([]*domain.Execution, int64, error) {
	var models []WorkflowExecutionModel
	var total int64

	var queryArgs []interface{}
	switch v := args.(type) {
	case []interface{}:
		queryArgs = v
	default:
		queryArgs = []interface{}{v}
	}

	q := r.db.WithContext(ctx).Model(&WorkflowExecutionModel{}).Where(where, queryArgs...)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("created_at DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}

	items := make([]*domain.Execution, len(models))
	for i := range models {
		items[i] = r.toDomain(&models[i])
	}
	return items, total, nil
}

func (r *WorkflowExecutionRepo) toDomain(m *WorkflowExecutionModel) *domain.Execution {
	return &domain.Execution{
		ID:           m.ID,
		WorkflowID:   m.WorkflowID,
		TriggerType:  domain.TriggerType(m.TriggerType),
		TriggerID:    m.TriggerID,
		InputText:    m.InputText,
		FinalText:    m.FinalText,
		Status:       domain.ExecutionStatus(m.Status),
		ErrorMessage: m.ErrorMessage,
		StartedAt:    m.StartedAt,
		CompletedAt:  m.CompletedAt,
		CreatedAt:    m.CreatedAt,
	}
}

// ─── NodeResultRepo ─────────────────────────────────────

type WorkflowNodeResultRepo struct {
	db *gorm.DB
}

func NewWorkflowNodeResultRepo(db *gorm.DB) *WorkflowNodeResultRepo {
	return &WorkflowNodeResultRepo{db: db}
}

func (r *WorkflowNodeResultRepo) BatchCreate(ctx context.Context, results []domain.NodeResult) error {
	if len(results) == 0 {
		return nil
	}
	models := make([]WorkflowNodeResultModel, len(results))
	for i, nr := range results {
		detailJSON := nr.Detail
		if detailJSON == "" {
			detailJSON = "{}"
		}
		// Validate JSON
		if !json.Valid([]byte(detailJSON)) {
			detailJSON = "{}"
		}
		models[i] = WorkflowNodeResultModel{
			ExecutionID: nr.ExecutionID,
			NodeID:      nr.NodeID,
			NodeType:    string(nr.NodeType),
			Position:    nr.Position,
			InputText:   nr.InputText,
			OutputText:  nr.OutputText,
			Status:      string(nr.Status),
			Detail:      detailJSON,
			DurationMs:  nr.DurationMs,
			ExecutedAt:  nr.ExecutedAt,
		}
	}
	return r.db.WithContext(ctx).Create(&models).Error
}

func (r *WorkflowNodeResultRepo) ListByExecution(ctx context.Context, executionID uint64) ([]domain.NodeResult, error) {
	var models []WorkflowNodeResultModel
	if err := r.db.WithContext(ctx).Where("execution_id = ?", executionID).Order("position ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]domain.NodeResult, len(models))
	for i, m := range models {
		items[i] = domain.NodeResult{
			ID:          m.ID,
			ExecutionID: m.ExecutionID,
			NodeID:      m.NodeID,
			NodeType:    domain.NodeType(m.NodeType),
			Position:    m.Position,
			InputText:   m.InputText,
			OutputText:  m.OutputText,
			Status:      domain.NodeResultStatus(m.Status),
			Detail:      m.Detail,
			DurationMs:  m.DurationMs,
			ExecutedAt:  m.ExecutedAt,
		}
	}
	return items, nil
}
