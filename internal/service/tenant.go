// Package service — SaaS tenant management service.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// =============================================================================
// Repository interface
// =============================================================================

// TenantRepo is the DB interface required by TenantService.
type TenantRepo interface {
	InsertTenant(ctx context.Context, t *domain.Tenant) error
	GetTenantByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error)
	GetTenantByDomain(ctx context.Context, domain string) (*domain.Tenant, error)
	ListTenants(ctx context.Context) ([]domain.Tenant, error)
	UpdateTenant(ctx context.Context, t *domain.Tenant) error
	AddUserToTenant(ctx context.Context, tu domain.TenantUser) error
	ListUsersForTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.TenantUser, error)
	GetSettings(ctx context.Context, tenantID uuid.UUID) (*domain.TenantSettings, error)
	UpsertSettings(ctx context.Context, ts *domain.TenantSettings) error
	TenantExists(ctx context.Context, tenantID uuid.UUID) (bool, error)
}

// =============================================================================
// Service
// =============================================================================

// TenantService manages tenant lifecycle and multi-tenancy primitives.
type TenantService struct {
	repo TenantRepo
	log  *zap.Logger
}

// NewTenantService creates a TenantService.
func NewTenantService(repo TenantRepo, log *zap.Logger) *TenantService {
	return &TenantService{repo: repo, log: log}
}

// =============================================================================
// Tenant CRUD
// =============================================================================

// CreateTenantInput is the request body for creating a tenant.
type CreateTenantInput struct {
	Name   string  `json:"name"`
	Domain *string `json:"domain,omitempty"`
	Plan   string  `json:"plan"`
}

// CreateTenant provisions a new tenant.
func (s *TenantService) CreateTenant(ctx context.Context, in CreateTenantInput) (*domain.Tenant, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("CreateTenant: name is required")
	}
	if in.Plan == "" {
		in.Plan = "starter"
	}

	t := &domain.Tenant{
		Name:     in.Name,
		Domain:   in.Domain,
		Plan:     in.Plan,
		IsActive: true,
	}
	if err := s.repo.InsertTenant(ctx, t); err != nil {
		return nil, fmt.Errorf("CreateTenant: %w", err)
	}

	// Seed empty settings row for the new tenant.
	_ = s.repo.UpsertSettings(ctx, &domain.TenantSettings{
		TenantID: t.ID,
		Settings: map[string]any{"currency": "AED", "vat_rate": "0.05"},
	})

	s.log.Info("tenant.created",
		zap.String("tenant_id", t.ID.String()),
		zap.String("name", t.Name),
	)
	return t, nil
}

// GetTenant returns a tenant by ID.
func (s *TenantService) GetTenant(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	return s.repo.GetTenantByID(ctx, id)
}

// GetTenantByDomain resolves a tenant from a domain / subdomain key.
// Returns the default tenant if domain is empty.
func (s *TenantService) GetTenantByDomain(ctx context.Context, domain string) (*domain.Tenant, error) {
	if domain == "" {
		return s.repo.GetTenantByID(ctx, domainDefaultTenantID)
	}
	t, err := s.repo.GetTenantByDomain(ctx, domain)
	if err != nil {
		// Fall back to default tenant so single-store deploys always work.
		return s.repo.GetTenantByID(ctx, domainDefaultTenantID)
	}
	return t, nil
}

// domainDefaultTenantID is the canonical default tenant UUID.
var domainDefaultTenantID = domain.DefaultTenantID

// ListTenants returns all tenants.
func (s *TenantService) ListTenants(ctx context.Context) ([]domain.Tenant, error) {
	return s.repo.ListTenants(ctx)
}

// UpdateTenant persists mutable tenant fields.
func (s *TenantService) UpdateTenant(ctx context.Context, t *domain.Tenant) error {
	return s.repo.UpdateTenant(ctx, t)
}

// =============================================================================
// User membership
// =============================================================================

// AddUserInput is the request to add a user to a tenant.
type AddUserInput struct {
	TenantID uuid.UUID `json:"tenant_id"`
	UserID   uuid.UUID `json:"user_id"`
	Role     string    `json:"role"`
}

// AddUser adds a user to a tenant with the given role.
func (s *TenantService) AddUser(ctx context.Context, in AddUserInput) error {
	if in.Role == "" {
		in.Role = "member"
	}
	return s.repo.AddUserToTenant(ctx, domain.TenantUser{
		TenantID: in.TenantID,
		UserID:   in.UserID,
		Role:     in.Role,
		JoinedAt: time.Now().UTC(),
	})
}

// ListUsers returns all users belonging to a tenant.
func (s *TenantService) ListUsers(ctx context.Context, tenantID uuid.UUID) ([]domain.TenantUser, error) {
	return s.repo.ListUsersForTenant(ctx, tenantID)
}

// =============================================================================
// Settings
// =============================================================================

// GetSettings returns the settings for a tenant.
func (s *TenantService) GetSettings(ctx context.Context, tenantID uuid.UUID) (*domain.TenantSettings, error) {
	return s.repo.GetSettings(ctx, tenantID)
}

// SaveSettings persists tenant settings.
func (s *TenantService) SaveSettings(ctx context.Context, tenantID uuid.UUID, settings map[string]any) error {
	return s.repo.UpsertSettings(ctx, &domain.TenantSettings{
		TenantID: tenantID,
		Settings: settings,
	})
}
