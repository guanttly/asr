package user

// LoginRequest is the DTO for user login.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse contains the JWT token.
type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresIn int64  `json:"expires_in"`
}

// CreateUserRequest is the DTO for creating a user.
type CreateUserRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=32"`
	Password    string `json:"password" binding:"required,min=6"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role" binding:"required,oneof=admin user"`
}

// UpdateUserRequest is the DTO for updating user info.
type UpdateUserRequest struct {
	DisplayName string `json:"display_name"`
	Role        string `json:"role" binding:"omitempty,oneof=admin user"`
}

// UserResponse is the DTO returned to clients.
type UserResponse struct {
	ID          uint64 `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}
