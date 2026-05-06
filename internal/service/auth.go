// Package service: Authentication + RBAC
//
// AuthService handles:
//   - Login  : validates email/password, issues JWT access + refresh token pair
//   - Refresh : exchanges a valid refresh token for a new pair
//   - Logout  : revokes the refresh token from Redis
//   - Me      : returns the authenticated user profile
//
// Access tokens are short-lived (15 min) and self-contained (permissions embedded).
// Refresh tokens are opaque UUIDs stored in Redis with a 7-day TTL.
// Redis key: "rt:{refreshTokenID}" → userID string
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// =============================================================================
// Configuration & repository interfaces
// =============================================================================

// AuthConfig holds tunable parameters for the auth service.
type AuthConfig struct {
	JWTSecret      string
	AccessTokenTTL time.Duration // recommended: 15 min
	RefreshTokenTTL time.Duration // recommended: 7 days
}

// AuthUserRepo is the DB interface required by AuthService.
type AuthUserRepo interface {
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	InsertUser(ctx context.Context, tx pgx.Tx, u *domain.User) error
	UpdateUser(ctx context.Context, u *domain.User) error
	ListUsers(ctx context.Context) ([]domain.User, error)
	AssignRoleToUser(ctx context.Context, userID, roleID uuid.UUID) error
	RemoveRoleFromUser(ctx context.Context, userID, roleID uuid.UUID) error
	GetRoleByName(ctx context.Context, name string) (*domain.Role, error)
	GetPermissionsVersion(ctx context.Context, userID uuid.UUID) (int, error)
	IncrementPermissionsVersion(ctx context.Context, userID uuid.UUID) error
}

// =============================================================================
// JWT Claims
// =============================================================================

// Claims is the JWT payload embedded in every access token.
type Claims struct {
	UserID             uuid.UUID `json:"uid"`
	Email              string    `json:"email"`
	Permissions        []string  `json:"perms"`
	PermissionsVersion int       `json:"pv"`  // per-user; incremented on every RBAC change
	GlobalAuthVersion  int       `json:"gav"` // system-wide; INCR to revoke every token instantly
	jwt.RegisteredClaims
}

// HasPermission returns true if the claim set includes the named permission.
func (c *Claims) HasPermission(perm string) bool {
	for _, p := range c.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

// =============================================================================
// Service
// =============================================================================

// AuthService provides login, refresh, logout, and user management.
type AuthService struct {
	repo   AuthUserRepo
	rdb    *redis.Client
	cfg    AuthConfig
	pool   TxBeginner
	log    *zap.Logger
}

// NewAuthService creates a new AuthService.
func NewAuthService(
	repo AuthUserRepo,
	rdb *redis.Client,
	cfg AuthConfig,
	pool TxBeginner,
	log *zap.Logger,
) *AuthService {
	if cfg.AccessTokenTTL == 0 {
		cfg.AccessTokenTTL = 15 * time.Minute
	}
	if cfg.RefreshTokenTTL == 0 {
		cfg.RefreshTokenTTL = 7 * 24 * time.Hour
	}
	return &AuthService{repo: repo, rdb: rdb, cfg: cfg, pool: pool, log: log}
}

// =============================================================================
// Login
// =============================================================================

// LoginInput carries the credentials submitted by the client.
type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login validates credentials and issues a token pair on success.
// Returns a descriptive error for invalid credentials (no information leak).
func (s *AuthService) Login(ctx context.Context, in LoginInput) (*domain.TokenPair, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))

	user, err := s.repo.GetUserByEmail(ctx, in.Email)
	if err != nil {
		// Do NOT reveal whether the email exists
		s.log.Warn("auth.login_failed", zap.String("email", in.Email), zap.String("reason", "user_not_found"))
		return nil, errors.New("invalid email or password")
	}

	if !user.IsActive {
		s.log.Warn("auth.login_failed", zap.String("email", in.Email), zap.String("reason", "account_inactive"))
		return nil, errors.New("account is disabled")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)); err != nil {
		s.log.Warn("auth.login_failed", zap.String("email", in.Email), zap.String("reason", "wrong_password"))
		return nil, errors.New("invalid email or password")
	}

	pair, err := s.issueTokenPair(ctx, user)
	if err != nil {
		return nil, err
	}

	s.log.Info("auth.login_success",
		zap.String("user_id", user.ID.String()),
		zap.String("email", user.Email),
	)
	return pair, nil
}

// =============================================================================
// Refresh
// =============================================================================

// Refresh validates a refresh token and issues a new token pair.
// The old refresh token is revoked (rotation pattern).
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*domain.TokenPair, error) {
	rtKey := "rt:" + refreshToken

	userIDStr, err := s.rdb.Get(ctx, rtKey).Result()
	if err == redis.Nil {
		return nil, errors.New("refresh token expired or invalid")
	}
	if err != nil {
		return nil, fmt.Errorf("Refresh: redis: %w", err)
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, errors.New("refresh token invalid")
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Revoke old token before issuing new one (prevents replay)
	_ = s.rdb.Del(ctx, rtKey)

	pair, err := s.issueTokenPair(ctx, user)
	if err != nil {
		return nil, err
	}

	s.log.Info("auth.token_refreshed", zap.String("user_id", user.ID.String()))
	return pair, nil
}

// =============================================================================
// Logout
// =============================================================================

// Logout revokes the refresh token, invalidating the session server-side.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) {
	if refreshToken == "" {
		return
	}
	_ = s.rdb.Del(ctx, "rt:"+refreshToken)
	pfxLen := 8
	if len(refreshToken) < pfxLen {
		pfxLen = len(refreshToken)
	}
	s.log.Info("auth.logout", zap.String("refresh_token_prefix", refreshToken[:pfxLen]))
}

// =============================================================================
// User management
// =============================================================================

// CreateUserInput is the DTO for creating a new user.
type CreateUserInput struct {
	Email     string   `json:"email"`
	Password  string   `json:"password"`
	FullName  string   `json:"full_name"`
	RoleNames []string `json:"roles"` // e.g. ["warehouse", "cashier"]
}

// CreateUser hashes the password and inserts a user with the given roles.
func (s *AuthService) CreateUser(ctx context.Context, in CreateUserInput) (*domain.User, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("CreateUser: hash: %w", err)
	}

	user := &domain.User{
		ID:           uuid.New(),
		Email:        in.Email,
		PasswordHash: string(hash),
		FullName:     in.FullName,
		IsActive:     true,
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("CreateUser: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.repo.InsertUser(ctx, tx, user); err != nil {
		return nil, fmt.Errorf("CreateUser: insert: %w", err)
	}

	// Assign requested roles
	for _, roleName := range in.RoleNames {
		role, err := s.repo.GetRoleByName(ctx, roleName)
		if err != nil {
			return nil, fmt.Errorf("CreateUser: role %q: %w", roleName, err)
		}
		if err := s.repo.AssignRoleToUser(ctx, user.ID, role.ID); err != nil {
			return nil, fmt.Errorf("CreateUser: assign role: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("CreateUser: commit: %w", err)
	}

	s.log.Info("auth.user_created",
		zap.String("user_id", user.ID.String()),
		zap.String("email", user.Email),
		zap.Strings("roles", in.RoleNames),
	)

	// Return user without hash
	user.PasswordHash = ""
	return user, nil
}

// UpdateUser persists updated user fields.
func (s *AuthService) UpdateUser(ctx context.Context, u *domain.User) error {
	return s.repo.UpdateUser(ctx, u)
}

// AssignRole adds a named role to an existing user and immediately invalidates
// outstanding access tokens by bumping permissions_version.
func (s *AuthService) AssignRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	role, err := s.repo.GetRoleByName(ctx, roleName)
	if err != nil {
		return fmt.Errorf("AssignRole: %w", err)
	}
	if err := s.repo.AssignRoleToUser(ctx, userID, role.ID); err != nil {
		return fmt.Errorf("AssignRole: assign: %w", err)
	}
	if err := s.repo.IncrementPermissionsVersion(ctx, userID); err != nil {
		// Non-fatal: log and continue — RBAC change succeeded; version will
		// self-correct on next token refresh.
		s.log.Error("AssignRole: failed to increment permissions_version",
			zap.String("user_id", userID.String()), zap.Error(err))
	}
	s.bustPermissionsCache(ctx, userID)
	s.log.Info("auth.role_assigned",
		zap.String("user_id", userID.String()),
		zap.String("role", roleName),
	)
	return nil
}

// RemoveRole removes a named role from a user and immediately invalidates
// outstanding access tokens by bumping permissions_version.
func (s *AuthService) RemoveRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	role, err := s.repo.GetRoleByName(ctx, roleName)
	if err != nil {
		return fmt.Errorf("RemoveRole: %w", err)
	}
	if err := s.repo.RemoveRoleFromUser(ctx, userID, role.ID); err != nil {
		return fmt.Errorf("RemoveRole: remove: %w", err)
	}
	if err := s.repo.IncrementPermissionsVersion(ctx, userID); err != nil {
		s.log.Error("RemoveRole: failed to increment permissions_version",
			zap.String("user_id", userID.String()), zap.Error(err))
	}
	s.bustPermissionsCache(ctx, userID)
	s.log.Info("auth.role_removed",
		zap.String("user_id", userID.String()),
		zap.String("role", roleName),
	)
	return nil
}

// ListUsers returns all users without passwords.
func (s *AuthService) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.repo.ListUsers(ctx)
}

// GetMe returns the authenticated user profile.
func (s *AuthService) GetMe(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	u, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	u.PasswordHash = ""
	return u, nil
}

// ValidateAccessToken parses and validates a JWT access token string.
// Returns the embedded Claims on success.
func (s *AuthService) ValidateAccessToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}
	return claims, nil
}

// =============================================================================
// Permission version helpers
// =============================================================================

// permVersionCacheTTL is how long we cache the permissions_version in Redis.
// Short enough that a version bump takes effect quickly; long enough to reduce
// DB hits for high-traffic deployments.
const permVersionCacheTTL = 5 * time.Minute

// ErrPermissionsChanged is returned when the JWT's embedded permissions_version
// no longer matches the user's current version in the system.
var ErrPermissionsChanged = errors.New("permissions changed, please re-login")

// CheckPermissionVersion verifies that the version embedded in the JWT claims
// matches the user's current permissions_version.
// It uses a Redis cache (5-min TTL) to avoid a DB call on every request.
func (s *AuthService) CheckPermissionVersion(ctx context.Context, claims *Claims) error {
	current, err := s.getPermissionsVersion(ctx, claims.UserID)
	if err != nil {
		// Fail closed: if we cannot verify, reject the request.
		return fmt.Errorf("cannot verify permission version: %w", err)
	}
	if current != claims.PermissionsVersion {
		return ErrPermissionsChanged
	}
	return nil
}

// getPermissionsVersion returns the authoritative permissions_version for a user.
// It checks Redis first; on a cache miss it falls back to the DB and populates
// the cache.
func (s *AuthService) getPermissionsVersion(ctx context.Context, userID uuid.UUID) (int, error) {
	key := permVersionKey(userID)

	val, err := s.rdb.Get(ctx, key).Int()
	if err == nil {
		return val, nil // cache hit
	}
	if err != redis.Nil {
		// Redis is unavailable; log and fall through to DB.
		s.log.Warn("auth.perm_version.redis_error", zap.Error(err))
	}

	// DB fallback
	version, dbErr := s.repo.GetPermissionsVersion(ctx, userID)
	if dbErr != nil {
		return 0, dbErr
	}

	// Best-effort cache population — ignore Redis errors here.
	_ = s.rdb.Set(ctx, key, version, permVersionCacheTTL).Err()
	return version, nil
}

// bustPermissionsCache deletes the cached permissions_version for a user,
// forcing the next request to re-read from the DB.
func (s *AuthService) bustPermissionsCache(ctx context.Context, userID uuid.UUID) {
	_ = s.rdb.Del(ctx, permVersionKey(userID)).Err()
}

func permVersionKey(userID uuid.UUID) string {
	return "auth:perm_version:" + userID.String()
}

// =============================================================================
// Global auth version helpers
// =============================================================================

// globalAuthVersionKey is the single Redis key that holds the system-wide
// auth version. Incrementing it invalidates every outstanding access token
// across all users simultaneously — intended for security-breach scenarios.
const globalAuthVersionKey = "auth:global_version"

// ErrGlobalRevocation is returned when the JWT's global_auth_version no longer
// matches the system-wide version (i.e. RevokeAllTokens was called).
var ErrGlobalRevocation = errors.New("all sessions revoked, please re-login")

// CheckGlobalAuthVersion verifies that the global_auth_version embedded in the
// JWT matches the current system-wide version stored in Redis.
//
// If the Redis key is absent (e.g. fresh deployment), it defaults to 1, which
// matches every token issued since the feature was introduced.
//
// Fails closed on Redis errors — a connectivity problem returns an error
// (and the middleware will reject the request with 401) rather than granting
// access on an unverifiable claim.
func (s *AuthService) CheckGlobalAuthVersion(ctx context.Context, claims *Claims) error {
	current, err := s.getGlobalAuthVersion(ctx)
	if err != nil {
		return fmt.Errorf("cannot verify global auth version: %w", err)
	}
	if claims.GlobalAuthVersion != current {
		return ErrGlobalRevocation
	}
	return nil
}

// RevokeAllTokens atomically increments the global auth version in Redis,
// immediately invalidating every outstanding access token system-wide.
// Returns the new version number so the caller can log it for auditing.
func (s *AuthService) RevokeAllTokens(ctx context.Context) (int64, error) {
	newVersion, err := s.rdb.Incr(ctx, globalAuthVersionKey).Result()
	if err != nil {
		return 0, fmt.Errorf("RevokeAllTokens: %w", err)
	}
	s.log.Warn("auth.global_revocation_triggered",
		zap.Int64("new_global_version", newVersion),
	)
	return newVersion, nil
}

// getGlobalAuthVersion reads the system-wide auth version from Redis.
// Returns 1 (the baseline) when the key does not exist so that tokens issued
// before this feature was deployed (which embed gav=1) remain valid.
func (s *AuthService) getGlobalAuthVersion(ctx context.Context) (int, error) {
	val, err := s.rdb.Get(ctx, globalAuthVersionKey).Int()
	if err == redis.Nil {
		return 1, nil // key absent → default baseline
	}
	if err != nil {
		return 0, fmt.Errorf("getGlobalAuthVersion: redis: %w", err)
	}
	return val, nil
}

// =============================================================================
// Internal helpers
// =============================================================================

func (s *AuthService) issueTokenPair(ctx context.Context, user *domain.User) (*domain.TokenPair, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(s.cfg.AccessTokenTTL)

	// Fetch global version; fall back to 1 on error so token issuance is never
	// blocked by a transient Redis hiccup.
	gav, err := s.getGlobalAuthVersion(ctx)
	if err != nil {
		s.log.Warn("auth.issue_token.global_version_unavailable", zap.Error(err))
		gav = 1
	}

	claims := &Claims{
		UserID:             user.ID,
		Email:              user.Email,
		Permissions:        user.Permissions,
		PermissionsVersion: user.PermissionsVersion,
		GlobalAuthVersion:  gav,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        uuid.New().String(),
		},
	}

	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return nil, fmt.Errorf("issueTokenPair: sign access: %w", err)
	}

	// Refresh token = opaque UUID, stored in Redis
	refreshToken := uuid.New().String()
	if err := s.rdb.Set(ctx, "rt:"+refreshToken, user.ID.String(), s.cfg.RefreshTokenTTL).Err(); err != nil {
		return nil, fmt.Errorf("issueTokenPair: redis: %w", err)
	}

	return &domain.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}

