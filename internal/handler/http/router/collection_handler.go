package router

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dubai-retail/os/internal/middleware"
	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type collectionHandler struct {
	svc *service.CollectionService
	log *zap.Logger
}

func newCollectionHandler(svc *service.CollectionService, log *zap.Logger) *collectionHandler {
	return &collectionHandler{svc: svc, log: log}
}

// POST /admin/collections
func (h *collectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == uuid.Nil {
		jsonError(w, "tenant not found", http.StatusUnauthorized)
		return
	}
	var input service.UpsertCollectionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	col, err := h.svc.CreateCollection(r.Context(), tenantID, input)
	if err != nil {
		h.log.Error("CreateCollection failed", zap.Error(err))
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(col)
}

// GET /admin/collections
func (h *collectionHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == uuid.Nil {
		jsonError(w, "tenant not found", http.StatusUnauthorized)
		return
	}
	list, err := h.svc.ListCollections(r.Context(), tenantID)
	if err != nil {
		h.log.Error("ListCollections failed", zap.Error(err))
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, list)
}

// GET /admin/collections/{id}
func (h *collectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == uuid.Nil {
		jsonError(w, "tenant not found", http.StatusUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid collection id", http.StatusBadRequest)
		return
	}
	col, err := h.svc.GetCollection(r.Context(), tenantID, id)
	if err != nil {
		jsonError(w, "collection not found", http.StatusNotFound)
		return
	}
	jsonOK(w, col)
}

// PATCH /admin/collections/{id}
func (h *collectionHandler) Patch(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == uuid.Nil {
		jsonError(w, "tenant not found", http.StatusUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid collection id", http.StatusBadRequest)
		return
	}
	var input service.UpsertCollectionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	col, err := h.svc.PatchCollection(r.Context(), tenantID, id, input)
	if err != nil {
		h.log.Error("PatchCollection failed", zap.Error(err))
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, col)
}

// DELETE /admin/collections/{id}
func (h *collectionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == uuid.Nil {
		jsonError(w, "tenant not found", http.StatusUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid collection id", http.StatusBadRequest)
		return
	}
	if err := h.svc.DeleteCollection(r.Context(), tenantID, id); err != nil {
		h.log.Error("DeleteCollection failed", zap.Error(err))
		jsonError(w, "failed to delete collection", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listPublicCollectionsHandler returns storefront collection metadata for nav / sections.
func listPublicCollectionsHandler(coll *service.CollectionService, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.TenantFromContext(r.Context())
		if tenantID == uuid.Nil {
			jsonError(w, "tenant not found", http.StatusUnauthorized)
			return
		}
		list, err := coll.ListCollections(r.Context(), tenantID)
		if err != nil {
			log.Error("store ListCollections failed", zap.Error(err))
			jsonError(w, "failed to list collections", http.StatusInternalServerError)
			return
		}

		type row struct {
			ID           uuid.UUID `json:"id"`
			Title        string    `json:"title"`
			Slug         string    `json:"slug"`
			Description  *string   `json:"description,omitempty"`
			ImageURL     *string   `json:"image_url,omitempty"`
			ProductCount int       `json:"product_count"`
		}
		out := make([]row, 0, len(list))
		for _, c := range list {
			if c == nil {
				continue
			}
			out = append(out, row{
				ID:           c.ID,
				Title:        c.Title,
				Slug:         c.Slug,
				Description:  c.Description,
				ImageURL:     c.ImageURL,
				ProductCount: c.ProductCount,
			})
		}
		jsonOK(w, out)
	}
}

func resolveCollectionSlugToID(r *http.Request, coll *service.CollectionService) (*uuid.UUID, error) {
	slugRaw := strings.TrimSpace(r.URL.Query().Get("collection_slug"))
	slug := strings.TrimSpace(strings.Trim(slugRaw, "/"))
	if slug == "" {
		return nil, nil
	}
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == uuid.Nil || coll == nil {
		return nil, nil
	}
	c, err := coll.GetCollectionBySlug(r.Context(), tenantID, slug)
	if err != nil {
		return nil, err
	}
	id := c.ID
	return &id, nil
}
