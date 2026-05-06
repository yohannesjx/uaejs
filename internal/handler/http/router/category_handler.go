package router

import (
	"encoding/json"
	"net/http"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/middleware"
	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type categoryHandler struct {
	svc *service.CategoryService
	log *zap.Logger
}

func newCategoryHandler(svc *service.CategoryService, log *zap.Logger) *categoryHandler {
	return &categoryHandler{svc: svc, log: log}
}

// POST /admin/products/categories
func (h *categoryHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == uuid.Nil {
		jsonError(w, "tenant not found", http.StatusUnauthorized)
		return
	}

	var input service.CreateCategoryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	cat, err := h.svc.CreateCategory(r.Context(), tenantID, input)
	if err != nil {
		h.log.Error("CreateCategory failed", zap.Error(err))
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(cat)
}

// GET /admin/products/categories
func (h *categoryHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == uuid.Nil {
		jsonError(w, "tenant not found", http.StatusUnauthorized)
		return
	}

	cats, err := h.svc.ListCategories(r.Context(), tenantID)
	if err != nil {
		h.log.Error("ListCategories failed", zap.Error(err))
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	if cats == nil {
		cats = make([]*domain.ProductCategory, 0) // ensure we return [] instead of null
	}
	jsonOK(w, cats)
}

// GET /admin/products/categories/{id}
func (h *categoryHandler) GetCategory(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == uuid.Nil {
		jsonError(w, "tenant not found", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid category id", http.StatusBadRequest)
		return
	}

	cat, err := h.svc.GetCategory(r.Context(), tenantID, id)
	if err != nil {
		h.log.Error("GetCategory failed", zap.Error(err))
		jsonError(w, "category not found", http.StatusNotFound)
		return
	}

	jsonOK(w, cat)
}

// DELETE /admin/products/categories/{id}
func (h *categoryHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == uuid.Nil {
		jsonError(w, "tenant not found", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid category id", http.StatusBadRequest)
		return
	}

	if err := h.svc.DeleteCategory(r.Context(), tenantID, id); err != nil {
		h.log.Error("DeleteCategory failed", zap.Error(err))
		jsonError(w, "failed to delete category", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PATCH /admin/products/categories/{id}
func (h *categoryHandler) PatchCategory(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == uuid.Nil {
		jsonError(w, "tenant not found", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid category id", http.StatusBadRequest)
		return
	}

	var input service.CreateCategoryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	cat, err := h.svc.PatchCategory(r.Context(), tenantID, id, input)
	if err != nil {
		h.log.Error("PatchCategory failed", zap.Error(err))
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	jsonOK(w, cat)
}
