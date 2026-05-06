package router

import (
	"encoding/json"
	"net/http"

	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type tenantHandler struct {
	svc *service.TenantService
}

func newTenantHandler(svc *service.TenantService) *tenantHandler {
	return &tenantHandler{svc: svc}
}

// GET /admin/tenants
func (h *tenantHandler) List(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.svc.ListTenants(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenants)
}

// POST /admin/tenants
func (h *tenantHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in service.CreateTenantInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	tenant, err := h.svc.CreateTenant(r.Context(), in)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tenant)
}

// GET /admin/tenants/{id}
func (h *tenantHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid tenant id"}`, http.StatusBadRequest)
		return
	}
	tenant, err := h.svc.GetTenant(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenant)
}

// GET /admin/tenants/{id}/settings
func (h *tenantHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid tenant id"}`, http.StatusBadRequest)
		return
	}
	settings, err := h.svc.GetSettings(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

// PUT /admin/tenants/{id}/settings
func (h *tenantHandler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid tenant id"}`, http.StatusBadRequest)
		return
	}
	var settings map[string]any
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if err := h.svc.SaveSettings(r.Context(), id, settings); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /admin/tenants/{id}/users
func (h *tenantHandler) AddUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid tenant id"}`, http.StatusBadRequest)
		return
	}
	var in service.AddUserInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	in.TenantID = id
	if err := h.svc.AddUser(r.Context(), in); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
