package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthRepository handles all user / role / permission DB operations.
type AuthRepository struct {
	pool *pgxpool.Pool
}

// ── Users ─────────────────────────────────────────────────────────────────────

// GetUserByEmail fetches a user with all their permissions pre-loaded.
func (r *AuthRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.loadUser(ctx, "WHERE u.email = $1", email)
}

// GetUserByID fetches a user by primary key.
func (r *AuthRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return r.loadUser(ctx, "WHERE u.id = $1", id)
}

// loadUser executes the user query with the supplied WHERE clause (one param).
func (r *AuthRepository) loadUser(ctx context.Context, where string, arg interface{}) (*domain.User, error) {
	q := fmt.Sprintf(`
		SELECT u.id, u.email, u.password_hash, u.full_name, u.is_active,
		       u.permissions_version, u.created_at, u.updated_at
		  FROM users u
		  %s`, where)

	u := &domain.User{}
	if err := r.pool.QueryRow(ctx, q, arg).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.IsActive,
		&u.PermissionsVersion, &u.CreatedAt, &u.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("loadUser: %w", err)
	}

	// Load flattened permissions via role chain
	perms, err := r.GetPermissionsForUser(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.Permissions = perms
	return u, nil
}

// InsertUser creates a new user row. permissions_version starts at 1.
func (r *AuthRepository) InsertUser(ctx context.Context, tx pgx.Tx, u *domain.User) error {
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now
	u.PermissionsVersion = 1
	_, err := tx.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, full_name, is_active, permissions_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 1, $6, $6)`,
		u.ID, u.Email, u.PasswordHash, u.FullName, u.IsActive, now,
	)
	return err
}

// UpdateUser updates mutable user fields.
func (r *AuthRepository) UpdateUser(ctx context.Context, u *domain.User) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET full_name=$2, is_active=$3, updated_at=NOW()
		 WHERE id=$1`, u.ID, u.FullName, u.IsActive)
	return err
}

// ListUsers returns all users (no passwords).
func (r *AuthRepository) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, email, full_name, is_active, created_at, updated_at
		  FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// ── Roles & permissions ───────────────────────────────────────────────────────

// GetPermissionsForUser returns the flat list of permission names for a user
// via the user_roles → role_permissions → permissions chain.
func (r *AuthRepository) GetPermissionsForUser(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT p.name
		  FROM permissions p
		  JOIN role_permissions rp ON rp.permission_id = p.id
		  JOIN user_roles       ur ON ur.role_id = rp.role_id
		 WHERE ur.user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("GetPermissionsForUser: %w", err)
	}
	defer rows.Close()
	var perms []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

// AssignRoleToUser adds a role to a user (idempotent via ON CONFLICT).
func (r *AuthRepository) AssignRoleToUser(ctx context.Context, userID, roleID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, roleID)
	return err
}

// RemoveRoleFromUser deletes a user→role mapping.
func (r *AuthRepository) RemoveRoleFromUser(ctx context.Context, userID, roleID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`,
		userID, roleID)
	return err
}

// GetPermissionsVersion returns the current permissions_version for the user.
// Used as a DB fallback when the Redis cache is cold.
func (r *AuthRepository) GetPermissionsVersion(ctx context.Context, userID uuid.UUID) (int, error) {
	var v int
	err := r.pool.QueryRow(ctx,
		`SELECT permissions_version FROM users WHERE id = $1`, userID,
	).Scan(&v)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, fmt.Errorf("user not found")
		}
		return 0, fmt.Errorf("GetPermissionsVersion: %w", err)
	}
	return v, nil
}

// IncrementPermissionsVersion atomically bumps permissions_version by 1.
// Call this after any RBAC mutation (assign/remove role) to immediately
// invalidate all outstanding access tokens for the user.
func (r *AuthRepository) IncrementPermissionsVersion(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET permissions_version = permissions_version + 1 WHERE id = $1`,
		userID)
	return err
}

// GetRoleByName returns a role by its string name.
func (r *AuthRepository) GetRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	var role domain.Role
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, description FROM roles WHERE name = $1`, name,
	).Scan(&role.ID, &role.Name, &role.Description)
	if err != nil {
		return nil, fmt.Errorf("GetRoleByName %q: %w", name, err)
	}
	return &role, nil
}

// ListRoles returns all roles.
func (r *AuthRepository) ListRoles(ctx context.Context) ([]domain.Role, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, description FROM roles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []domain.Role
	for rows.Next() {
		var rl domain.Role
		if err := rows.Scan(&rl.ID, &rl.Name, &rl.Description); err != nil {
			return nil, err
		}
		roles = append(roles, rl)
	}
	return roles, rows.Err()
}
