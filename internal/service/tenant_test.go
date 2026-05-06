package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// =============================================================================
// Fake repository
// =============================================================================

type fakeTenantRepo struct {
	tenants      map[uuid.UUID]*domain.Tenant
	byDomain     map[string]*domain.Tenant
	users        map[uuid.UUID][]domain.TenantUser
	settings     map[uuid.UUID]*domain.TenantSettings
}

func newFakeTenantRepo() *fakeTenantRepo {
	defaultTenant := &domain.Tenant{
		ID:       domain.DefaultTenantID,
		Name:     "Default Store",
		Plan:     "enterprise",
		IsActive: true,
	}
	return &fakeTenantRepo{
		tenants:  map[uuid.UUID]*domain.Tenant{domain.DefaultTenantID: defaultTenant},
		byDomain: map[string]*domain.Tenant{},
		users:    map[uuid.UUID][]domain.TenantUser{},
		settings: map[uuid.UUID]*domain.TenantSettings{
			domain.DefaultTenantID: {
				TenantID: domain.DefaultTenantID,
				Settings: map[string]any{"currency": "AED"},
			},
		},
	}
}

func (r *fakeTenantRepo) InsertTenant(_ context.Context, t *domain.Tenant) error {
	t.ID = uuid.New()
	t.CreatedAt = time.Now()
	t.UpdatedAt = t.CreatedAt
	r.tenants[t.ID] = t
	if t.Domain != nil {
		r.byDomain[*t.Domain] = t
	}
	return nil
}

func (r *fakeTenantRepo) GetTenantByID(_ context.Context, id uuid.UUID) (*domain.Tenant, error) {
	t, ok := r.tenants[id]
	if !ok {
		return nil, fmt.Errorf("tenant not found")
	}
	return t, nil
}

func (r *fakeTenantRepo) GetTenantByDomain(_ context.Context, d string) (*domain.Tenant, error) {
	t, ok := r.byDomain[d]
	if !ok {
		return nil, fmt.Errorf("tenant not found for domain %s", d)
	}
	return t, nil
}

func (r *fakeTenantRepo) ListTenants(_ context.Context) ([]domain.Tenant, error) {
	var out []domain.Tenant
	for _, t := range r.tenants {
		out = append(out, *t)
	}
	return out, nil
}

func (r *fakeTenantRepo) UpdateTenant(_ context.Context, t *domain.Tenant) error {
	r.tenants[t.ID] = t
	return nil
}

func (r *fakeTenantRepo) AddUserToTenant(_ context.Context, tu domain.TenantUser) error {
	r.users[tu.TenantID] = append(r.users[tu.TenantID], tu)
	return nil
}

func (r *fakeTenantRepo) ListUsersForTenant(_ context.Context, id uuid.UUID) ([]domain.TenantUser, error) {
	return r.users[id], nil
}

func (r *fakeTenantRepo) GetSettings(_ context.Context, id uuid.UUID) (*domain.TenantSettings, error) {
	ts, ok := r.settings[id]
	if !ok {
		return &domain.TenantSettings{TenantID: id, Settings: map[string]any{}}, nil
	}
	return ts, nil
}

func (r *fakeTenantRepo) UpsertSettings(_ context.Context, ts *domain.TenantSettings) error {
	r.settings[ts.TenantID] = ts
	return nil
}

func (r *fakeTenantRepo) TenantExists(_ context.Context, id uuid.UUID) (bool, error) {
	_, ok := r.tenants[id]
	return ok, nil
}

// =============================================================================
// Tests
// =============================================================================

func TestTenant_Create_Success(t *testing.T) {
	svc := service.NewTenantService(newFakeTenantRepo(), zap.NewNop())
	ctx := context.Background()

	tenant, err := svc.CreateTenant(ctx, service.CreateTenantInput{
		Name: "Boutique Dubai",
		Plan: "pro",
	})
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	if tenant.ID == uuid.Nil {
		t.Error("expected non-nil tenant ID")
	}
	if tenant.Name != "Boutique Dubai" {
		t.Errorf("name mismatch: %s", tenant.Name)
	}
	if tenant.Plan != "pro" {
		t.Errorf("plan mismatch: %s", tenant.Plan)
	}
	if !tenant.IsActive {
		t.Error("new tenant should be active")
	}
}

func TestTenant_Create_DefaultPlan(t *testing.T) {
	svc := service.NewTenantService(newFakeTenantRepo(), zap.NewNop())
	tenant, err := svc.CreateTenant(context.Background(), service.CreateTenantInput{Name: "New Store"})
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	if tenant.Plan != "starter" {
		t.Errorf("expected default plan 'starter', got %q", tenant.Plan)
	}
}

func TestTenant_Create_EmptyName_Rejected(t *testing.T) {
	svc := service.NewTenantService(newFakeTenantRepo(), zap.NewNop())
	_, err := svc.CreateTenant(context.Background(), service.CreateTenantInput{Name: ""})
	if err == nil {
		t.Error("expected error for empty tenant name")
	}
}

func TestTenant_Isolation_DifferentTenantsSeeOnlyTheirData(t *testing.T) {
	repo := newFakeTenantRepo()
	svc := service.NewTenantService(repo, zap.NewNop())
	ctx := context.Background()

	// Create two tenants
	t1, _ := svc.CreateTenant(ctx, service.CreateTenantInput{Name: "Store A"})
	t2, _ := svc.CreateTenant(ctx, service.CreateTenantInput{Name: "Store B"})

	// Add a user to each tenant
	user1 := uuid.New()
	user2 := uuid.New()
	_ = svc.AddUser(ctx, service.AddUserInput{TenantID: t1.ID, UserID: user1, Role: "owner"})
	_ = svc.AddUser(ctx, service.AddUserInput{TenantID: t2.ID, UserID: user2, Role: "owner"})

	// Tenant 1 should only see its own user
	users1, _ := svc.ListUsers(ctx, t1.ID)
	for _, u := range users1 {
		if u.UserID == user2 {
			t.Error("tenant 1 should not see tenant 2's user — cross-tenant data leakage!")
		}
	}

	// Tenant 2 should only see its own user
	users2, _ := svc.ListUsers(ctx, t2.ID)
	for _, u := range users2 {
		if u.UserID == user1 {
			t.Error("tenant 2 should not see tenant 1's user — cross-tenant data leakage!")
		}
	}
}

func TestTenant_CrossAccess_Denied(t *testing.T) {
	repo := newFakeTenantRepo()
	svc := service.NewTenantService(repo, zap.NewNop())
	ctx := context.Background()

	t1, _ := svc.CreateTenant(ctx, service.CreateTenantInput{Name: "Tenant Alpha"})

	// Add settings to tenant 1
	_ = svc.SaveSettings(ctx, t1.ID, map[string]any{"secret_key": "alpha-secret"})

	// Create tenant 2 and try to read tenant 1's settings via its own scoped call
	t2, _ := svc.CreateTenant(ctx, service.CreateTenantInput{Name: "Tenant Beta"})
	_ = svc.SaveSettings(ctx, t2.ID, map[string]any{"secret_key": "beta-secret"})

	// Tenant 2's settings must not leak tenant 1's secret
	s2, err := svc.GetSettings(ctx, t2.ID)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s2.Settings["secret_key"] == "alpha-secret" {
		t.Error("cross-tenant settings leakage detected")
	}
	if s2.Settings["secret_key"] != "beta-secret" {
		t.Errorf("tenant 2 should see its own settings, got %v", s2.Settings["secret_key"])
	}
}

func TestTenant_GetByDomain_Resolves(t *testing.T) {
	repo := newFakeTenantRepo()
	svc := service.NewTenantService(repo, zap.NewNop())
	ctx := context.Background()

	domain := "myboutique"
	domainPtr := &domain
	tenant, _ := svc.CreateTenant(ctx, service.CreateTenantInput{Name: "My Boutique", Domain: domainPtr})

	resolved, err := svc.GetTenantByDomain(ctx, "myboutique")
	if err != nil {
		t.Fatalf("GetTenantByDomain: %v", err)
	}
	if resolved.ID != tenant.ID {
		t.Errorf("expected tenant %s, got %s", tenant.ID, resolved.ID)
	}
}

func TestTenant_GetByDomain_FallsBackToDefault(t *testing.T) {
	svc := service.NewTenantService(newFakeTenantRepo(), zap.NewNop())
	// No tenant registered for "unknown" domain → should return default
	resolved, err := svc.GetTenantByDomain(context.Background(), "unknown-domain")
	if err != nil {
		t.Fatalf("expected fallback to default tenant, got error: %v", err)
	}
	if resolved.ID != domain.DefaultTenantID {
		t.Errorf("expected default tenant, got %s", resolved.ID)
	}
}

func TestTenant_Settings_Upsert(t *testing.T) {
	svc := service.NewTenantService(newFakeTenantRepo(), zap.NewNop())
	ctx := context.Background()

	tenant, _ := svc.CreateTenant(ctx, service.CreateTenantInput{Name: "Test Store"})

	if err := svc.SaveSettings(ctx, tenant.ID, map[string]any{"currency": "USD", "vat_rate": "0"}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	settings, err := svc.GetSettings(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings.Settings["currency"] != "USD" {
		t.Errorf("expected currency USD, got %v", settings.Settings["currency"])
	}
}
