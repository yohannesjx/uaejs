package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/dubai-retail/os/internal/middleware"
	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type productHandler struct {
	svc *service.ProductService
	log *zap.Logger
}

func newProductHandler(svc *service.ProductService, log *zap.Logger) *productHandler {
	return &productHandler{svc: svc, log: log}
}

// =============================================================================
// POST /api/v1/products
// =============================================================================

func (h *productHandler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var input service.CreateProductInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	input.TenantID = middleware.TenantFromContext(r.Context())

	result, err := h.svc.CreateProductWithVariants(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDuplicateSKU):
			jsonError(w, err.Error(), http.StatusConflict)
		case strings.Contains(strings.ToLower(err.Error()), "duplicate sku in request"):
			jsonError(w, err.Error(), http.StatusBadRequest)
		default:
			h.log.Error("CreateProduct failed", zap.Error(err))
			jsonError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(result)
}

// =============================================================================
// GET /api/v1/products/{id}
// =============================================================================

func (h *productHandler) GetProduct(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid product id", http.StatusBadRequest)
		return
	}

	result, err := h.svc.GetProduct(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProductNotFound):
			jsonError(w, "product not found", http.StatusNotFound)
		default:
			h.log.Error("GetProduct failed", zap.Error(err))
			jsonError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	jsonOK(w, result)
}

// =============================================================================
// PUT /api/v1/products/{id}/prices
// Body: { "variant_id", "channel_id", "price", "currency", "effective_until" }
// =============================================================================

func (h *productHandler) SetPrice(w http.ResponseWriter, r *http.Request) {
	var input service.SetPriceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	cp, err := h.svc.SetPrice(r.Context(), input)
	if err != nil {
		h.log.Error("SetPrice failed", zap.Error(err))
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	jsonOK(w, cp)
}

// =============================================================================
// GET /api/v1/products/{id}/prices?channel_id=...&variant_id=...
// =============================================================================

func (h *productHandler) GetPrice(w http.ResponseWriter, r *http.Request) {
	variantID, err := uuid.Parse(r.URL.Query().Get("variant_id"))
	if err != nil {
		jsonError(w, "invalid variant_id", http.StatusBadRequest)
		return
	}
	channelID, err := uuid.Parse(r.URL.Query().Get("channel_id"))
	if err != nil {
		jsonError(w, "invalid channel_id", http.StatusBadRequest)
		return
	}

	cp, err := h.svc.GetPrice(r.Context(), variantID, channelID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	jsonOK(w, cp)
}

// =============================================================================
// POST /admin/products/drafts
// =============================================================================

func (h *productHandler) CreateDraft(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.CreateDraft(r.Context())
	if err != nil {
		h.log.Error("CreateDraft failed", zap.Error(err))
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(result)
}

// =============================================================================
// PATCH /admin/products/{id}
// =============================================================================

func (h *productHandler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid product id", http.StatusBadRequest)
		return
	}

	var input service.UpdateProductInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.UpdateProduct(r.Context(), id, input); err != nil {
		h.log.Error("UpdateProduct failed", zap.Error(err))
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *productHandler) UpsertProductDefaultVariant(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	productID, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid product id", http.StatusBadRequest)
		return
	}
	var input service.UpsertVariantInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	v, err := h.svc.UpsertDefaultVariantForProduct(r.Context(), productID, input)
	if err != nil {
		h.log.Error("UpsertProductDefaultVariant failed", zap.Error(err))
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, v)
}

func (h *productHandler) CreateProductVariant(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	productID, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid product id", http.StatusBadRequest)
		return
	}
	var input service.UpsertVariantInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	v, err := h.svc.CreateVariantForProduct(r.Context(), productID, input)
	if err != nil {
		h.log.Error("CreateProductVariant failed", zap.Error(err))
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, v)
}

func (h *productHandler) PatchVariant(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	variantID, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid variant id", http.StatusBadRequest)
		return
	}
	var input service.UpsertVariantInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.svc.UpdateVariant(r.Context(), variantID, input); err != nil {
		h.log.Error("PatchVariant failed", zap.Error(err))
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *productHandler) DeleteVariant(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	variantID, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid variant id", http.StatusBadRequest)
		return
	}
	if err := h.svc.DeleteVariant(r.Context(), variantID); err != nil {
		h.log.Error("DeleteVariant failed", zap.Error(err))
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *productHandler) DuplicateProduct(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	productID, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid product id", http.StatusBadRequest)
		return
	}
	p, err := h.svc.DuplicateProduct(r.Context(), productID)
	if err != nil {
		h.log.Error("DuplicateProduct failed", zap.Error(err))
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, p)
}

func (h *productHandler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	productID, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid product id", http.StatusBadRequest)
		return
	}
	if err := h.svc.DeleteProduct(r.Context(), productID); err != nil {
		h.log.Error("DeleteProduct failed", zap.Error(err))
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}
