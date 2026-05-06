package domain

import (
	"time"

	"github.com/google/uuid"
)

// DefaultTenantID is the well-known UUID assigned to the single-store
// deployment. All pre-migration rows carry this ID.
var DefaultTenantID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// Tenant represents an isolated store / business unit on the platform.
type Tenant struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Domain    *string   `json:"domain,omitempty"`
	Plan      string    `json:"plan"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TenantUser is the many-to-many link between a tenant and a user.
type TenantUser struct {
	TenantID uuid.UUID `json:"tenant_id"`
	UserID   uuid.UUID `json:"user_id"`
	Role     string    `json:"role"` // owner / admin / member
	JoinedAt time.Time `json:"joined_at"`
}

// TenantSettings holds arbitrary per-tenant configuration.
type TenantSettings struct {
	TenantID  uuid.UUID      `json:"tenant_id"`
	Settings  map[string]any `json:"settings"`
	UpdatedAt time.Time      `json:"updated_at"`
}
