package user

import "time"

// Role defines user permission level.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// User represents a system user.
type User struct {
	ID           uint64    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	DisplayName  string    `json:"display_name"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DeviceIdentity records the machine fingerprint bound to an anonymous desktop user.
type DeviceIdentity struct {
	ID           uint64    `json:"id"`
	UserID       uint64    `json:"user_id"`
	MachineCode  string    `json:"machine_code"`
	Hostname     string    `json:"hostname"`
	Platform     string    `json:"platform"`
	IPAddresses  []string  `json:"ip_addresses"`
	MACAddresses []string  `json:"mac_addresses"`
	LastSeenAt   time.Time `json:"last_seen_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
