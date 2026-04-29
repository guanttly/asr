package openplatform

import "context"

type AppRepository interface {
	Create(ctx context.Context, app *App) error
	GetByID(ctx context.Context, id uint64) (*App, error)
	GetByAppID(ctx context.Context, appID string) (*App, error)
	GetByName(ctx context.Context, name string) (*App, error)
	Update(ctx context.Context, app *App) error
	UpdateStatus(ctx context.Context, id uint64, status AppStatus) error
	List(ctx context.Context, offset, limit int) ([]*App, int64, error)
}

type SkillRepository interface {
	Create(ctx context.Context, skill *Skill) error
	GetByID(ctx context.Context, id uint64) (*Skill, error)
	GetByUID(ctx context.Context, uid string) (*Skill, error)
	GetByAppAndName(ctx context.Context, appID uint64, name string) (*Skill, error)
	Update(ctx context.Context, skill *Skill) error
	Delete(ctx context.Context, id uint64) error
	ListByApp(ctx context.Context, appID uint64) ([]*Skill, error)
}

type CallLogRepository interface {
	Create(ctx context.Context, call *CallLog) error
	ListByApp(ctx context.Context, appID uint64, limit int) ([]*CallLog, error)
}

type SkillInvocationRepository interface {
	Create(ctx context.Context, invocation *SkillInvocation) error
	ListBySkill(ctx context.Context, skillID uint64, limit int) ([]*SkillInvocation, error)
}
