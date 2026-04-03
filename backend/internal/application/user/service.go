package user

import (
	"context"
	"errors"

	domain "github.com/lgt/asr/internal/domain/user"
	"golang.org/x/crypto/bcrypt"
)

// Service orchestrates user use cases.
type Service struct {
	userRepo domain.UserRepository
}

// NewService creates a new user application service.
func NewService(userRepo domain.UserRepository) *Service {
	return &Service{userRepo: userRepo}
}

// CreateUser registers a new user.
func (s *Service) CreateUser(ctx context.Context, req *CreateUserRequest) (*UserResponse, error) {
	role := domain.Role(req.Role)
	if role != domain.RoleAdmin && role != domain.RoleUser {
		return nil, errors.New("invalid role")
	}

	if _, err := s.userRepo.GetByUsername(ctx, req.Username); err == nil {
		return nil, domain.ErrUserAlreadyExists
	} else if !errors.Is(err, domain.ErrUserNotFound) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		Username:     req.Username,
		PasswordHash: string(hash),
		DisplayName:  req.DisplayName,
		Role:         role,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return toResponse(user), nil
}

// Authenticate verifies credentials and returns the user.
func (s *Service) Authenticate(ctx context.Context, username, password string) (*domain.User, error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}

	return user, nil
}

// GetUser retrieves user by ID.
func (s *Service) GetUser(ctx context.Context, id uint64) (*UserResponse, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return toResponse(user), nil
}

// ListUsers returns a paginated list of users.
func (s *Service) ListUsers(ctx context.Context, offset, limit int) ([]*UserResponse, int64, error) {
	users, total, err := s.userRepo.List(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}

	items := make([]*UserResponse, len(users))
	for i, user := range users {
		items[i] = toResponse(user)
	}

	return items, total, nil
}

// EnsureAdmin ensures the bootstrap admin account exists.
func (s *Service) EnsureAdmin(ctx context.Context, username, password, displayName string) error {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err == nil {
		if user.Role != domain.RoleAdmin {
			return errors.New("bootstrap username exists but is not admin")
		}
		return nil
	}
	if !errors.Is(err, domain.ErrUserNotFound) {
		return err
	}

	_, err = s.CreateUser(ctx, &CreateUserRequest{
		Username:    username,
		Password:    password,
		DisplayName: displayName,
		Role:        string(domain.RoleAdmin),
	})
	if errors.Is(err, domain.ErrUserAlreadyExists) {
		return nil
	}
	return err
}

func toResponse(user *domain.User) *UserResponse {
	return &UserResponse{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        string(user.Role),
	}
}
