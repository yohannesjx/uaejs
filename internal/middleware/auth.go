// Package middleware provides HTTP middleware for JWT authentication and RBAC.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/dubai-retail/os/internal/service"
	"go.uber.org/zap"
)

type contextKey string

const claimsKey contextKey = "auth_claims"

// AuthMiddleware validates Bearer JWT tokens on protected routes.
type AuthMiddleware struct {
	auth *service.AuthService
	log  *zap.Logger
}

// New creates a new AuthMiddleware.
func New(auth *service.AuthService, log *zap.Logger) *AuthMiddleware {
	return &AuthMiddleware{auth: auth, log: log}
}

// Authenticate extracts and validates the access token.
// Token sources (in order):
//  1. Authorization: Bearer <token>
//  2. Cookie: access_token=<token>
// It performs two checks in sequence:
//  1. Cryptographic JWT validation (signature + expiry).
//  2. Permission-version check: the version embedded in the token must match
//     the user's current permissions_version (Redis-cached, DB fallback). If
//     a role has been added or removed since the token was issued, this check
//     fails and the client must re-login.
//
// On success it stores the claims in the request context.
// On failure it returns 401 Unauthorized.
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := extractBearerToken(r)
		if tokenStr == "" {
			http.Error(w, `{"error":"missing access token"}`, http.StatusUnauthorized)
			return
		}

		claims, err := m.auth.ValidateAccessToken(tokenStr)
		if err != nil {
			m.log.Debug("auth.token_invalid", zap.Error(err), zap.String("path", r.URL.Path))
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		// Global revocation check — one Redis GET for the entire system.
		// If RevokeAllTokens was ever called, this rejects every token issued
		// before that moment regardless of which user it belongs to.
		if err := m.auth.CheckGlobalAuthVersion(r.Context(), claims); err != nil {
			m.log.Warn("auth.global_version_mismatch",
				zap.String("user_id", claims.UserID.String()),
				zap.String("path", r.URL.Path),
				zap.Error(err),
			)
			http.Error(w, `{"error":"all sessions revoked, please re-login"}`, http.StatusUnauthorized)
			return
		}

		// Per-user permission version check — cached in Redis with DB fallback.
		// Rejects tokens issued before the last role assignment or removal.
		if err := m.auth.CheckPermissionVersion(r.Context(), claims); err != nil {
			m.log.Info("auth.permission_version_mismatch",
				zap.String("user_id", claims.UserID.String()),
				zap.String("path", r.URL.Path),
				zap.Error(err),
			)
			http.Error(w, `{"error":"permissions changed, please re-login"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequirePermission returns a middleware that requires the authenticated user
// to hold the named permission. Must be used AFTER Authenticate.
func RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
				return
			}
			if !claims.HasPermission(perm) {
				http.Error(w, `{"error":"forbidden","required_permission":"`+perm+`"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClaimsFromContext retrieves JWT claims stored by Authenticate middleware.
// Returns nil if the request was not authenticated.
func ClaimsFromContext(ctx context.Context) *service.Claims {
	v := ctx.Value(claimsKey)
	if v == nil {
		return nil
	}
	c, _ := v.(*service.Claims)
	return c
}

// extractBearerToken pulls token from Authorization Bearer header first,
// then falls back to access_token cookie for browser-based storefront requests.
func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h != "" {
		parts := strings.SplitN(h, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") && strings.TrimSpace(parts[1]) != "" {
			return parts[1]
		}
	}
	if c, err := r.Cookie("access_token"); err == nil {
		if strings.TrimSpace(c.Value) != "" {
			return c.Value
		}
	}
	return ""
}
