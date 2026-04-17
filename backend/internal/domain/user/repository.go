package user

import (
	"context"
	"errors"
)

var (
	// ErrUserNotFound indicates the requested user does not exist.
	ErrUserNotFound = errors.New("user not found")
	// ErrUserAlreadyExists indicates a duplicate username.
	ErrUserAlreadyExists = errors.New("user already exists")
	// ErrDeviceIdentityNotFound indicates the requested device identity does not exist.
	ErrDeviceIdentityNotFound = errors.New("device identity not found")
)

// UserRepository defines persistence operations for User.
type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uint64) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	GetDeviceIdentityByMachineCode(ctx context.Context, machineCode string) (*DeviceIdentity, error)
	GetWorkflowBindings(ctx context.Context, userID uint64) (*WorkflowBindings, error)
	UpsertDeviceIdentity(ctx context.Context, identity *DeviceIdentity) error
	SaveWorkflowBindings(ctx context.Context, bindings *WorkflowBindings) error
	Update(ctx context.Context, u *User) error
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context, offset, limit int) ([]*User, int64, error)
}
