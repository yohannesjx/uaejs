package domain

import (
	"time"

	"github.com/google/uuid"
)

// User represents a system user (admin, warehouse staff, cashier, etc.).
type User struct {
	ID                 uuid.UUID `json:"id"`
	Email              string    `json:"email"`
	PasswordHash       string    `json:"-"` // never serialised
	FullName           string    `json:"full_name"`
	IsActive           bool      `json:"is_active"`
	Roles              []Role    `json:"roles,omitempty"`
	Permissions        []string  `json:"permissions,omitempty"` // flattened from roles
	PermissionsVersion int       `json:"permissions_version"`   // incremented on every RBAC change
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Role maps to the roles table.
type Role struct {
	ID          uuid.UUID    `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Permissions []Permission `json:"permissions,omitempty"`
}

// Permission maps to the permissions table.
type Permission struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
}

// HasPermission returns true if the user holds the named permission.
func (u *User) HasPermission(perm string) bool {
	for _, p := range u.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

// TokenPair is issued on successful login or token refresh.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}
