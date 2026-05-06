package router

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type orderHandler struct {
	svc *service.OrderService
	log *zap.Logger
}

func newOrderHandler(svc *service.OrderService, log *zap.Logger) *orderHandler {
	return &orderHandler{svc: svc, log: log}
}

// =============================================================================
// POST /api/v1/orders
//
// Creates and confirms an order in one shot (POS flow).
// E-commerce pre-auth flow uses POST /api/v1/inventory/reserve first.
// =============================================================================

func (h *orderHandler) ProcessOrder(w http.ResponseWriter, r *http.Request) {
	var input service.ProcessOrderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		jsonError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	result, err := h.svc.ProcessOrder(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInsufficientStock):
			jsonError(w, err.Error(), http.StatusConflict)
		default:
			h.log.Error("ProcessOrder failed", zap.Error(err))
			jsonError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(result)
}

// =============================================================================
// GET /api/v1/orders/{id}
// =============================================================================

func (h *orderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, "invalid order id", http.StatusBadRequest)
		return
	}

	order, err := h.svc.GetOrder(r.Context(), id)
	if err != nil {
		h.log.Error("GetOrder failed", zap.Error(err))
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	jsonOK(w, order)
}
