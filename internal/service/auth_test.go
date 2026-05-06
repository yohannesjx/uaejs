package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// =============================================================================
// Fakes
// =============================================================================

type fakeAuthRepo struct {
	users    map[string]*domain.User // key = email
	byID     map[uuid.UUID]*domain.User
	roles    map[string]*domain.Role
	versions map[uuid.UUID]int // tracks permissions_version per user
}

func newFakeAuthRepo() *fakeAuthRepo {
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.MinCost)
	uid := uuid.New()
	u := &domain.User{
		ID:                 uid,
		Email:              "alice@example.com",
		PasswordHash:       string(hash),
		FullName:           "Alice Smith",
		IsActive:           true,
		Permissions:        []string{"orders.manage", "products.read"},
		PermissionsVersion: 1,
	}
	r := &fakeAuthRepo{
		users: map[string]*domain.User{"alice@example.com": u},
		byID:  map[uuid.UUID]*domain.User{uid: u},
		roles: map[string]*domain.Role{
			"cashier":   {ID: uuid.New(), Name: "cashier"},
			"warehouse": {ID: uuid.New(), Name: "warehouse"},
		},
		versions: map[uuid.UUID]int{uid: 1},
	}
	return r
}

func (r *fakeAuthRepo) GetUserByEmail(_ context.Context, email string) (*domain.User, error) {
	u := r.users[email]
	if u == nil {
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}
func (r *fakeAuthRepo) GetUserByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	u := r.byID[id]
	if u == nil {
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}
func (r *fakeAuthRepo) InsertUser(_ context.Context, _ pgx.Tx, u *domain.User) error {
	u.PermissionsVersion = 1
	r.users[u.Email] = u
	r.byID[u.ID] = u
	r.versions[u.ID] = 1
	return nil
}
func (r *fakeAuthRepo) UpdateUser(_ context.Context, u *domain.User) error {
	r.users[u.Email] = u
	r.byID[u.ID] = u
	return nil
}
func (r *fakeAuthRepo) ListUsers(_ context.Context) ([]domain.User, error) {
	var out []domain.User
	for _, u := range r.users {
		out = append(out, *u)
	}
	return out, nil
}
func (r *fakeAuthRepo) AssignRoleToUser(_ context.Context, _, _ uuid.UUID) error { return nil }
func (r *fakeAuthRepo) RemoveRoleFromUser(_ context.Context, _, _ uuid.UUID) error { return nil }
func (r *fakeAuthRepo) GetRoleByName(_ context.Context, name string) (*domain.Role, error) {
	rl := r.roles[name]
	if rl == nil {
		return nil, fmt.Errorf("role not found: %s", name)
	}
	return rl, nil
}
func (r *fakeAuthRepo) GetPermissionsVersion(_ context.Context, userID uuid.UUID) (int, error) {
	v, ok := r.versions[userID]
	if !ok {
		return 0, fmt.Errorf("user not found")
	}
	return v, nil
}
func (r *fakeAuthRepo) IncrementPermissionsVersion(_ context.Context, userID uuid.UUID) error {
	if _, ok := r.versions[userID]; !ok {
		return fmt.Errorf("user not found")
	}
	r.versions[userID]++
	// Keep the user struct in sync so GetUserByID returns the new version.
	if u, ok := r.byID[userID]; ok {
		u.PermissionsVersion = r.versions[userID]
	}
	return nil
}

// fakeRedis is a minimal in-memory Redis substitute for auth tests.
type fakeRedis struct {
	data map[string]string
	ttl  map[string]time.Time
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{data: make(map[string]string), ttl: make(map[string]time.Time)}
}

func (r *fakeRedis) asClient() *redis.Client {
	// We can't easily fake redis.Client; tests use the real token flow via the service.
	// Instead, we test indirectly by using a real Redis client pointing to a test Redis
	// or by extracting the token logic into a testable interface.
	// For unit tests we accept that the refresh-token Redis path is tested at integration level.
	return nil
}

// =============================================================================
// Test helpers
// =============================================================================

func newTestAuthService(t *testing.T) (*service.AuthService, *fakeAuthRepo) {
	t.Helper()

	// Use a real Redis client for token storage in tests (uses test Redis on port 6380)
	// If Redis is unavailable the token-related tests are skipped gracefully.
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6380",
		Password: "change_me_in_prod",
		DB:       1, // test DB
	})

	repo := newFakeAuthRepo()
	cfg := service.AuthConfig{
		JWTSecret:       "test_jwt_secret_32bytes_minimum!!",
		AccessTokenTTL:  1 * time.Hour,
		RefreshTokenTTL: 24 * time.Hour,
	}
	svc := service.NewAuthService(repo, rdb, cfg, newIntegrationFakeTxBeginner(), zap.NewNop())
	return svc, repo
}

func skipIfRedisUnavailable(t *testing.T, rdb *redis.Client) {
	t.Helper()
	if rdb == nil {
		t.Skip("Redis unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis ping failed: %v", err)
	}
}

// =============================================================================
// Tests
// =============================================================================

func TestAuth_Login_Success(t *testing.T) {
	svc, _ := newTestAuthService(t)

	pair, err := svc.Login(context.Background(), service.LoginInput{
		Email:    "alice@example.com",
		Password: "secret123",
	})
	if err != nil {
		t.Fatalf("Login should succeed: %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if pair.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if pair.ExpiresAt.IsZero() {
		t.Error("expected non-zero expiry")
	}
}

func TestAuth_Login_InvalidPassword(t *testing.T) {
	svc, _ := newTestAuthService(t)

	_, err := svc.Login(context.Background(), service.LoginInput{
		Email:    "alice@example.com",
		Password: "wrongpassword",
	})
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	// Must not reveal which field was wrong
	if err.Error() != "invalid email or password" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestAuth_Login_UnknownEmail(t *testing.T) {
	svc, _ := newTestAuthService(t)

	_, err := svc.Login(context.Background(), service.LoginInput{
		Email:    "ghost@example.com",
		Password: "any",
	})
	if err == nil {
		t.Fatal("expected error for unknown email")
	}
	if err.Error() != "invalid email or password" {
		t.Errorf("unexpected error message (must not reveal email existence): %s", err.Error())
	}
}

func TestAuth_ValidateAccessToken(t *testing.T) {
	svc, _ := newTestAuthService(t)

	pair, err := svc.Login(context.Background(), service.LoginInput{
		Email:    "alice@example.com",
		Password: "secret123",
	})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("token validation failed: %v", err)
	}
	if claims.Email != "alice@example.com" {
		t.Errorf("expected alice@example.com, got %s", claims.Email)
	}
}

func TestAuth_RBAC_PermissionAllowed(t *testing.T) {
	svc, _ := newTestAuthService(t)

	pair, _ := svc.Login(context.Background(), service.LoginInput{
		Email: "alice@example.com", Password: "secret123",
	})
	claims, _ := svc.ValidateAccessToken(pair.AccessToken)

	if !claims.HasPermission("orders.manage") {
		t.Error("alice should have orders.manage permission")
	}
}

func TestAuth_RBAC_PermissionDenied(t *testing.T) {
	svc, _ := newTestAuthService(t)

	pair, _ := svc.Login(context.Background(), service.LoginInput{
		Email: "alice@example.com", Password: "secret123",
	})
	claims, _ := svc.ValidateAccessToken(pair.AccessToken)

	if claims.HasPermission("analytics.view") {
		t.Error("alice should NOT have analytics.view permission")
	}
}

func TestAuth_RefreshToken_Flow(t *testing.T) {
	svc, _ := newTestAuthService(t)

	// Obtain initial pair
	pair, err := svc.Login(context.Background(), service.LoginInput{
		Email: "alice@example.com", Password: "secret123",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// Refresh
	newPair, err := svc.Refresh(context.Background(), pair.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if newPair.AccessToken == pair.AccessToken {
		t.Error("new access token should differ from old one")
	}
	if newPair.RefreshToken == pair.RefreshToken {
		t.Error("new refresh token should differ (rotation)")
	}

	// Old refresh token must now be invalid
	_, err = svc.Refresh(context.Background(), pair.RefreshToken)
	if err == nil {
		t.Error("old refresh token should be invalid after rotation")
	}
}

func TestAuth_InvalidToken_Rejected(t *testing.T) {
	svc, _ := newTestAuthService(t)

	_, err := svc.ValidateAccessToken("this.is.not.a.valid.jwt")
	if err == nil {
		t.Error("expected error for malformed token")
	}
}

// =============================================================================
// Permission Version Tests
// =============================================================================

// TestAuth_PermissionVersion_EmbeddedInToken verifies that the JWT access
// token contains a non-zero permissions_version claim.
func TestAuth_PermissionVersion_EmbeddedInToken(t *testing.T) {
	svc, _ := newTestAuthService(t)

	pair, err := svc.Login(context.Background(), service.LoginInput{
		Email: "alice@example.com", Password: "secret123",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.PermissionsVersion == 0 {
		t.Error("expected PermissionsVersion > 0 in JWT claims")
	}
}

// TestAuth_PermissionVersion_CheckValid verifies that a freshly issued token
// passes the permission-version check when no RBAC changes have occurred.
func TestAuth_PermissionVersion_CheckValid(t *testing.T) {
	svc, _ := newTestAuthService(t)

	ctx := context.Background()
	pair, err := svc.Login(ctx, service.LoginInput{
		Email: "alice@example.com", Password: "secret123",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	if err := svc.CheckPermissionVersion(ctx, claims); err != nil {
		t.Errorf("expected valid version check, got: %v", err)
	}
}

// TestAuth_PermissionVersion_RejectedAfterRoleAssigned verifies that assigning
// a role to a user increments the permissions_version and causes the old JWT
// to be rejected by CheckPermissionVersion.
func TestAuth_PermissionVersion_RejectedAfterRoleAssigned(t *testing.T) {
	svc, repo := newTestAuthService(t)

	ctx := context.Background()
	pair, err := svc.Login(ctx, service.LoginInput{
		Email: "alice@example.com", Password: "secret123",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	// Simulate assigning a new role (bumps permissions_version and busts cache).
	aliceID := claims.UserID
	if err := svc.AssignRole(ctx, aliceID, "warehouse"); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}

	// Confirm the repo version was incremented.
	newVer, _ := repo.GetPermissionsVersion(ctx, aliceID)
	if newVer != 2 {
		t.Errorf("expected permissions_version=2 after role change, got %d", newVer)
	}

	// The old token (version=1) should now be rejected.
	if err := svc.CheckPermissionVersion(ctx, claims); err == nil {
		t.Error("expected old token to be rejected after role assignment")
	}
}

// TestAuth_PermissionVersion_RejectedAfterRoleRemoved verifies that removing a
// role also increments the permissions_version, invalidating the old JWT.
func TestAuth_PermissionVersion_RejectedAfterRoleRemoved(t *testing.T) {
	svc, repo := newTestAuthService(t)

	ctx := context.Background()
	pair, err := svc.Login(ctx, service.LoginInput{
		Email: "alice@example.com", Password: "secret123",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	aliceID := claims.UserID
	if err := svc.RemoveRole(ctx, aliceID, "cashier"); err != nil {
		t.Fatalf("RemoveRole: %v", err)
	}

	newVer, _ := repo.GetPermissionsVersion(ctx, aliceID)
	if newVer != 2 {
		t.Errorf("expected permissions_version=2 after role removal, got %d", newVer)
	}

	if err := svc.CheckPermissionVersion(ctx, claims); err == nil {
		t.Error("expected old token to be rejected after role removal")
	}
}

// TestAuth_PermissionVersion_RefreshIssuesNewVersion verifies that after a role
// change the user can obtain a new token (via Refresh) that embeds the updated
// permissions_version and passes the version check.
func TestAuth_PermissionVersion_RefreshIssuesNewVersion(t *testing.T) {
	svc, _ := newTestAuthService(t)

	ctx := context.Background()
	pair, err := svc.Login(ctx, service.LoginInput{
		Email: "alice@example.com", Password: "secret123",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	// Bump the version by assigning a role.
	if err := svc.AssignRole(ctx, claims.UserID, "cashier"); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}

	// Old token is rejected.
	if err := svc.CheckPermissionVersion(ctx, claims); err == nil {
		t.Error("old token should be rejected after role change")
	}

	// Refresh to get a new pair that carries the updated version.
	newPair, err := svc.Refresh(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	newClaims, err := svc.ValidateAccessToken(newPair.AccessToken)
	if err != nil {
		t.Fatalf("validate new token: %v", err)
	}

	if newClaims.PermissionsVersion <= claims.PermissionsVersion {
		t.Errorf("new token should carry higher version (%d), got %d",
			claims.PermissionsVersion, newClaims.PermissionsVersion)
	}

	// New token passes the version check.
	if err := svc.CheckPermissionVersion(ctx, newClaims); err != nil {
		t.Errorf("new token should pass version check: %v", err)
	}
}

// =============================================================================
// Global Auth Version Tests
// =============================================================================

// TestAuth_GlobalVersion_EmbeddedInToken verifies that a freshly issued JWT
// contains a non-zero gav (global_auth_version) claim.
func TestAuth_GlobalVersion_EmbeddedInToken(t *testing.T) {
	svc, _ := newTestAuthService(t)

	pair, err := svc.Login(context.Background(), service.LoginInput{
		Email: "alice@example.com", Password: "secret123",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	// Default is 1 when key is absent; must be at least 1.
	if claims.GlobalAuthVersion < 1 {
		t.Errorf("expected GlobalAuthVersion >= 1, got %d", claims.GlobalAuthVersion)
	}
}

// TestAuth_GlobalVersion_CheckValid verifies that a fresh token passes the
// global version check when no revocation has occurred.
func TestAuth_GlobalVersion_CheckValid(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	pair, err := svc.Login(ctx, service.LoginInput{
		Email: "alice@example.com", Password: "secret123",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	if err := svc.CheckGlobalAuthVersion(ctx, claims); err != nil {
		t.Errorf("expected valid global version check, got: %v", err)
	}
}

// TestAuth_GlobalVersion_RevokeAll_InvalidatesAllTokens verifies that calling
// RevokeAllTokens increments the global version and causes all existing tokens
// to be rejected by CheckGlobalAuthVersion.
func TestAuth_GlobalVersion_RevokeAll_InvalidatesAllTokens(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6380",
		Password: "change_me_in_prod",
		DB:       1,
	})
	skipIfRedisUnavailable(t, rdb)
	defer rdb.Close()

	// Ensure a known baseline by resetting the global version to 1.
	if err := rdb.Set(ctx, "auth:global_version", 1, 0).Err(); err != nil {
		t.Fatalf("seed global version: %v", err)
	}

	pair, err := svc.Login(ctx, service.LoginInput{
		Email: "alice@example.com", Password: "secret123",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	// Token should be valid before revocation.
	if err := svc.CheckGlobalAuthVersion(ctx, claims); err != nil {
		t.Fatalf("pre-revocation check should pass: %v", err)
	}

	// Trigger global revocation.
	newVer, err := svc.RevokeAllTokens(ctx)
	if err != nil {
		t.Fatalf("RevokeAllTokens: %v", err)
	}
	if newVer != 2 {
		t.Errorf("expected global version 2 after first revocation, got %d", newVer)
	}

	// The old token (gav=1) must now be rejected.
	if err := svc.CheckGlobalAuthVersion(ctx, claims); err == nil {
		t.Error("expected old token to be rejected after global revocation")
	}
}

// TestAuth_GlobalVersion_NewLoginAfterRevocation verifies that a user who
// re-logs in after a global revocation receives a token with the updated
// global version that passes the check.
func TestAuth_GlobalVersion_NewLoginAfterRevocation(t *testing.T) {
	svc, _ := newTestAuthService(t)
	ctx := context.Background()

	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6380",
		Password: "change_me_in_prod",
		DB:       1,
	})
	skipIfRedisUnavailable(t, rdb)
	defer rdb.Close()

	// Set a known global version = 3 (simulating past revocations).
	if err := rdb.Set(ctx, "auth:global_version", 3, 0).Err(); err != nil {
		t.Fatalf("seed global version: %v", err)
	}

	pair, err := svc.Login(ctx, service.LoginInput{
		Email: "alice@example.com", Password: "secret123",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	if claims.GlobalAuthVersion != 3 {
		t.Errorf("expected GlobalAuthVersion=3, got %d", claims.GlobalAuthVersion)
	}

	if err := svc.CheckGlobalAuthVersion(ctx, claims); err != nil {
		t.Errorf("new token with current global version should pass: %v", err)
	}

	// Reset so other tests are not affected.
	_ = rdb.Set(ctx, "auth:global_version", 1, 0).Err()
}

// TestAuth_GlobalVersion_TableDriven runs a concise set of global-version check
// scenarios without requiring Redis, by directly testing the comparison logic.
func TestAuth_GlobalVersion_TableDriven(t *testing.T) {
	tests := []struct {
		name          string
		tokenVersion  int
		globalVersion int
		wantErr       bool
	}{
		{"matching version → allowed", 1, 1, false},
		{"matching version 5 → allowed", 5, 5, false},
		{"token before first revocation → rejected", 1, 2, true},
		{"token multiple revocations behind → rejected", 1, 10, true},
		{"token ahead of global (impossible, but safe) → rejected", 3, 2, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newTestAuthService(t)
			ctx := context.Background()

			rdb := redis.NewClient(&redis.Options{
				Addr:     "localhost:6380",
				Password: "change_me_in_prod",
				DB:       1,
			})
			skipIfRedisUnavailable(t, rdb)

			// Force the global version to the desired value.
			if err := rdb.Set(ctx, "auth:global_version", tc.globalVersion, 0).Err(); err != nil {
				t.Fatalf("seed: %v", err)
			}
			rdb.Close()

			testClaims := &service.Claims{
				GlobalAuthVersion:  tc.tokenVersion,
				PermissionsVersion: 1,
			}

			err := svc.CheckGlobalAuthVersion(ctx, testClaims)
			if tc.wantErr && err == nil {
				t.Error("expected global version mismatch error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}

	// Restore a clean baseline for subsequent test runs.
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6380",
		Password: "change_me_in_prod",
		DB:       1,
	})
	_ = rdb.Set(context.Background(), "auth:global_version", 1, 0).Err()
	rdb.Close()
}

// TestAuth_PermissionVersion_TableDriven runs a concise set of version-check
// scenarios in a table-driven style.
func TestAuth_PermissionVersion_TableDriven(t *testing.T) {
	tests := []struct {
		name          string
		tokenVersion  int
		currentVersion int
		wantErr       bool
	}{
		{
			name:           "matching version → allowed",
			tokenVersion:   1,
			currentVersion: 1,
			wantErr:        false,
		},
		{
			name:           "token behind by 1 → rejected",
			tokenVersion:   1,
			currentVersion: 2,
			wantErr:        true,
		},
		{
			name:           "token behind by many → rejected",
			tokenVersion:   1,
			currentVersion: 5,
			wantErr:        true,
		},
		{
			name:           "version 0 (legacy token) with current=1 → rejected",
			tokenVersion:   0,
			currentVersion: 1,
			wantErr:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newTestAuthService(t)
			ctx := context.Background()

			// Get alice's real user ID.
			pair, err := svc.Login(ctx, service.LoginInput{
				Email: "alice@example.com", Password: "secret123",
			})
			if err != nil {
				t.Fatalf("login: %v", err)
			}
			realClaims, _ := svc.ValidateAccessToken(pair.AccessToken)
			userID := realClaims.UserID

			// Force the repo to the desired current version.
			repo.versions[userID] = tc.currentVersion
			if u, ok := repo.byID[userID]; ok {
				u.PermissionsVersion = tc.currentVersion
			}

			// Craft claims with the desired token version (no Redis involved).
			testClaims := &service.Claims{
				UserID:             userID,
				Email:              "alice@example.com",
				PermissionsVersion: tc.tokenVersion,
			}

			err = svc.CheckPermissionVersion(ctx, testClaims)
			if tc.wantErr && err == nil {
				t.Error("expected version mismatch error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}
