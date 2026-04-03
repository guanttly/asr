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
)

// UserRepository defines persistence operations for User.
type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uint64) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	Update(ctx context.Context, u *User) error
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context, offset, limit int) ([]*User, int64, error)
}
