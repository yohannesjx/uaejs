package router

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dubai-retail/os/internal/middleware"
	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type authHandler struct {
	auth *service.AuthService
}

func newAuthHandler(auth *service.AuthService) *authHandler {
	return &authHandler{auth: auth}
}

// POST /auth/login
func (h *authHandler) Login(w http.ResponseWriter, r *http.Request) {
	var in service.LoginInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(in.Email) == "" || strings.TrimSpace(in.Password) == "" {
		http.Error(w, `{"error":"email and password are required"}`, http.StatusBadRequest)
		return
	}

	pair, err := h.auth.Login(r.Context(), in)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pair)
}

// POST /auth/refresh
func (h *authHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RefreshToken == "" {
		http.Error(w, `{"error":"refresh_token is required"}`, http.StatusBadRequest)
		return
	}

	pair, err := h.auth.Refresh(r.Context(), body.RefreshToken)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pair)
}

// POST /auth/logout
func (h *authHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	h.auth.Logout(r.Context(), body.RefreshToken)
	w.WriteHeader(http.StatusNoContent)
}

// GET /auth/me
func (h *authHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	user, err := h.auth.GetMe(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// ── Admin user management ─────────────────────────────────────────────────────

// GET /admin/users
func (h *authHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.auth.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// POST /admin/users
func (h *authHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var in service.CreateUserInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	user, err := h.auth.CreateUser(r.Context(), in)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

// PATCH /admin/users/{id}
func (h *authHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid user id"}`, http.StatusBadRequest)
		return
	}
	var body struct {
		FullName *string `json:"full_name"`
		IsActive *bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	user, err := h.auth.GetMe(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}
	if body.FullName != nil {
		user.FullName = *body.FullName
	}
	if body.IsActive != nil {
		user.IsActive = *body.IsActive
	}
	if err := h.auth.UpdateUser(r.Context(), user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// POST /admin/auth/revoke-all
// Requires: users.manage permission.
// Increments the global auth version in Redis, immediately invalidating every
// outstanding access token system-wide. Use during a security breach or after
// a credential compromise. All users must re-login to obtain a new token.
func (h *authHandler) RevokeAll(w http.ResponseWriter, r *http.Request) {
	newVersion, err := h.auth.RevokeAllTokens(r.Context())
	if err != nil {
		http.Error(w, `{"error":"failed to revoke sessions"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":             "all sessions revoked — users must re-login",
		"global_auth_version": newVersion,
	})
}

// POST /admin/users/{id}/roles
func (h *authHandler) AssignRole(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid user id"}`, http.StatusBadRequest)
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Role == "" {
		http.Error(w, `{"error":"role is required"}`, http.StatusBadRequest)
		return
	}
	if err := h.auth.AssignRole(r.Context(), id, body.Role); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
