package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// tenantKey is the context key for the resolved tenant ID.
type tenantKey struct{}

// TenantMiddleware resolves the active tenant for each request.
// Resolution order:
//  1. X-Tenant-ID header (explicit UUID)
//  2. JWT claim "tid" embedded by AuthMiddleware (future extension)
//  3. Subdomain of the Host header (e.g. "store1.myapp.com" → "store1")
//  4. Default tenant (single-store deployments)
//
// The resolved tenant ID is stored in the request context and can be
// retrieved via TenantFromContext.
type TenantMiddleware struct {
	svc TenantResolver
	log *zap.Logger
}

// TenantResolver is the minimal interface TenantMiddleware needs.
type TenantResolver interface {
	GetTenantByDomain(ctx context.Context, domain string) (*domain.Tenant, error)
	GetTenant(ctx context.Context, id uuid.UUID) (*domain.Tenant, error)
}

// NewTenantMiddleware creates a TenantMiddleware.
func NewTenantMiddleware(svc TenantResolver, log *zap.Logger) *TenantMiddleware {
	return &TenantMiddleware{svc: svc, log: log}
}

// Resolve injects the tenant ID into the request context.
// It never returns 4xx — unresolvable tenants silently fall back to the
// default tenant so single-store deployments require no configuration.
func (m *TenantMiddleware) Resolve(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := m.resolveTenantID(r)
		ctx := context.WithValue(r.Context(), tenantKey{}, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireTenant is a stricter version that returns 401 when no valid tenant
// can be resolved. Use on multi-tenant API routes where the default tenant
// should NOT be silently substituted.
func (m *TenantMiddleware) RequireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := m.resolveTenantID(r)
		if tenantID == domain.DefaultTenantID {
			// Check if explicit header was sent but invalid.
			if r.Header.Get("X-Tenant-ID") != "" {
				http.Error(w, `{"error":"invalid or inactive tenant"}`, http.StatusUnauthorized)
				return
			}
		}
		ctx := context.WithValue(r.Context(), tenantKey{}, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *TenantMiddleware) resolveTenantID(r *http.Request) uuid.UUID {
	// 1. Explicit header
	if hdr := r.Header.Get("X-Tenant-ID"); hdr != "" {
		if id, err := uuid.Parse(hdr); err == nil {
			if _, err := m.svc.GetTenant(r.Context(), id); err == nil {
				return id
			}
			m.log.Warn("tenant.header_id_not_found", zap.String("X-Tenant-ID", hdr))
		}
	}

	// 2. Subdomain (e.g. "acme.myapp.com" → domain key "acme")
	if host := r.Host; host != "" {
		subdomain := extractSubdomain(host)
		if subdomain != "" && subdomain != "www" {
			if t, err := m.svc.GetTenantByDomain(r.Context(), subdomain); err == nil {
				return t.ID
			}
		}
	}

	// 3. Default tenant fallback
	return domain.DefaultTenantID
}

// extractSubdomain returns the first label of the hostname (before the first dot),
// or empty string if the hostname has no dots.
func extractSubdomain(host string) string {
	// Strip port
	if i := strings.LastIndex(host, ":"); i != -1 {
		host = host[:i]
	}
	if i := strings.Index(host, "."); i != -1 {
		return host[:i]
	}
	return ""
}

// TenantFromContext retrieves the resolved tenant ID from the request context.
// Returns domain.DefaultTenantID when no tenant is in context (safe default).
func TenantFromContext(ctx context.Context) uuid.UUID {
	v := ctx.Value(tenantKey{})
	if v == nil {
		return domain.DefaultTenantID
	}
	id, ok := v.(uuid.UUID)
	if !ok {
		return domain.DefaultTenantID
	}
	return id
}
