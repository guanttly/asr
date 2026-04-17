package user

import "encoding/json"

// LoginRequest is the DTO for user login.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse contains the JWT token.
type LoginResponse struct {
	Token     string        `json:"token"`
	ExpiresIn int64         `json:"expires_in"`
	User      *UserResponse `json:"user,omitempty"`
}

// AnonymousLoginRequest is the DTO for machine-code based desktop login.
type AnonymousLoginRequest struct {
	MachineCode  string   `json:"machine_code" binding:"required,min=16,max=128"`
	DisplayName  string   `json:"display_name" binding:"max=128"`
	Hostname     string   `json:"hostname" binding:"max=255"`
	Platform     string   `json:"platform" binding:"max=64"`
	IPAddresses  []string `json:"ip_addresses"`
	MACAddresses []string `json:"mac_addresses"`
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

// UpdateProfileRequest updates the current user's display name.
type UpdateProfileRequest struct {
	DisplayName string `json:"display_name" binding:"required,max=128"`
}

// UserResponse is the DTO returned to clients.
type UserResponse struct {
	ID          uint64 `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// MarshalJSON keeps empty slices stable when echoing anonymous login payloads in logs/tests.
func (r AnonymousLoginRequest) MarshalJSON() ([]byte, error) {
	type alias AnonymousLoginRequest
	copyReq := alias(r)
	if copyReq.IPAddresses == nil {
		copyReq.IPAddresses = []string{}
	}
	if copyReq.MACAddresses == nil {
		copyReq.MACAddresses = []string{}
	}
	return json.Marshal(copyReq)
}

// UpdateWorkflowBindingsRequest updates default app workflow bindings for the current user.
type UpdateWorkflowBindingsRequest struct {
	Realtime *uint64 `json:"realtime"`
	Batch    *uint64 `json:"batch"`
	Meeting  *uint64 `json:"meeting"`
}

// WorkflowBindingsResponse returns default app workflow bindings for the current user.
type WorkflowBindingsResponse struct {
	Realtime *uint64 `json:"realtime,omitempty"`
	Batch    *uint64 `json:"batch,omitempty"`
	Meeting  *uint64 `json:"meeting,omitempty"`
}
