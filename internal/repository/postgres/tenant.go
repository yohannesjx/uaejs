package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TenantRepository handles all tenant CRUD operations.
type TenantRepository struct {
	pool *pgxpool.Pool
}

// InsertTenant creates a new tenant.
func (r *TenantRepository) InsertTenant(ctx context.Context, t *domain.Tenant) error {
	t.ID = uuid.New()
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tenants (id, name, domain, plan, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		t.ID, t.Name, t.Domain, t.Plan, t.IsActive, now,
	)
	return err
}

// GetTenantByID returns a tenant by primary key.
func (r *TenantRepository) GetTenantByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	return r.loadTenant(ctx, `WHERE id = $1`, id)
}

// GetTenantByDomain returns a tenant by its domain key.
func (r *TenantRepository) GetTenantByDomain(ctx context.Context, domain string) (*domain.Tenant, error) {
	return r.loadTenant(ctx, `WHERE domain = $1`, domain)
}

func (r *TenantRepository) loadTenant(ctx context.Context, where string, arg any) (*domain.Tenant, error) {
	q := fmt.Sprintf(`SELECT id, name, domain, plan, is_active, created_at, updated_at FROM tenants %s`, where)
	var t domain.Tenant
	err := r.pool.QueryRow(ctx, q, arg).Scan(
		&t.ID, &t.Name, &t.Domain, &t.Plan, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("tenant not found")
	}
	if err != nil {
		return nil, fmt.Errorf("loadTenant: %w", err)
	}
	return &t, nil
}

// ListTenants returns all tenants.
func (r *TenantRepository) ListTenants(ctx context.Context) ([]domain.Tenant, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, domain, plan, is_active, created_at, updated_at FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tenants []domain.Tenant
	for rows.Next() {
		var t domain.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Domain, &t.Plan, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

// UpdateTenant persists mutable tenant fields.
func (r *TenantRepository) UpdateTenant(ctx context.Context, t *domain.Tenant) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tenants SET name = $2, domain = $3, plan = $4, is_active = $5, updated_at = NOW() WHERE id = $1`,
		t.ID, t.Name, t.Domain, t.Plan, t.IsActive,
	)
	return err
}

// AddUserToTenant creates a tenant_users link.
func (r *TenantRepository) AddUserToTenant(ctx context.Context, tu domain.TenantUser) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tenant_users (tenant_id, user_id, role, joined_at)
		 VALUES ($1, $2, $3, $4) ON CONFLICT (tenant_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		tu.TenantID, tu.UserID, tu.Role, tu.JoinedAt,
	)
	return err
}

// ListUsersForTenant returns all users belonging to a tenant.
func (r *TenantRepository) ListUsersForTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.TenantUser, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT tenant_id, user_id, role, joined_at FROM tenant_users WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []domain.TenantUser
	for rows.Next() {
		var tu domain.TenantUser
		if err := rows.Scan(&tu.TenantID, &tu.UserID, &tu.Role, &tu.JoinedAt); err != nil {
			return nil, err
		}
		users = append(users, tu)
	}
	return users, rows.Err()
}

// GetSettings returns the settings for a tenant.
func (r *TenantRepository) GetSettings(ctx context.Context, tenantID uuid.UUID) (*domain.TenantSettings, error) {
	var ts domain.TenantSettings
	var raw []byte
	err := r.pool.QueryRow(ctx,
		`SELECT tenant_id, settings, updated_at FROM tenant_settings WHERE tenant_id = $1`, tenantID,
	).Scan(&ts.TenantID, &raw, &ts.UpdatedAt)
	if err == pgx.ErrNoRows {
		return &domain.TenantSettings{TenantID: tenantID, Settings: map[string]any{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetSettings: %w", err)
	}
	_ = json.Unmarshal(raw, &ts.Settings)
	return &ts, nil
}

// UpsertSettings saves tenant settings.
func (r *TenantRepository) UpsertSettings(ctx context.Context, ts *domain.TenantSettings) error {
	raw, _ := json.Marshal(ts.Settings)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tenant_settings (tenant_id, settings, updated_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (tenant_id) DO UPDATE SET settings = EXCLUDED.settings, updated_at = NOW()`,
		ts.TenantID, raw,
	)
	return err
}

// TenantExists returns true if the tenant ID exists in the DB.
func (r *TenantRepository) TenantExists(ctx context.Context, tenantID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1 AND is_active = TRUE)`, tenantID,
	).Scan(&exists)
	return exists, err
}
