package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/dubai-retail/os/internal/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type inventoryHandler struct {
	svc *service.InventoryService
	log *zap.Logger
}

func newInventoryHandler(svc *service.InventoryService, log *zap.Logger) *inventoryHandler {
	return &inventoryHandler{svc: svc, log: log}
}

// =============================================================================
// POST /api/v1/inventory/deduct
// =============================================================================

type deductRequest struct {
	Items []struct {
		VariantID uuid.UUID `json:"variant_id"`
		OrderID   uuid.UUID `json:"order_id"`
		ChannelID uuid.UUID `json:"channel_id"`
		Quantity  int       `json:"quantity"`
	} `json:"items"`
}

func (h *inventoryHandler) DeductStock(w http.ResponseWriter, r *http.Request) {
	var req deductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	items := make([]service.DeductionItem, 0, len(req.Items))
	for _, it := range req.Items {
		if it.Quantity <= 0 {
			jsonError(w, "quantity must be positive", http.StatusBadRequest)
			return
		}
		items = append(items, service.DeductionItem{
			VariantID: it.VariantID,
			OrderID:   it.OrderID,
			ChannelID: it.ChannelID,
			Quantity:  it.Quantity,
		})
	}

	results, err := h.svc.SubtractStock(r.Context(), items)
	if err != nil {
		if errors.Is(err, service.ErrInsufficientStock) {
			jsonError(w, err.Error(), http.StatusConflict)
			return
		}
		h.log.Error("DeductStock failed", zap.Error(err))
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, results)
}

// =============================================================================
// POST /api/v1/inventory/reserve
// =============================================================================

type reserveRequest struct {
	ReservationTTLSeconds int `json:"reservation_ttl_seconds"`
	Items                 []struct {
		OrderID   uuid.UUID `json:"order_id"`
		VariantID uuid.UUID `json:"variant_id"`
		ChannelID uuid.UUID `json:"channel_id"`
		Quantity  int       `json:"quantity"`
	} `json:"items"`
}

func (h *inventoryHandler) ReserveStock(w http.ResponseWriter, r *http.Request) {
	var req reserveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ttl := time.Duration(req.ReservationTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	requests := make([]service.ReserveRequest, 0, len(req.Items))
	for _, it := range req.Items {
		if it.Quantity <= 0 {
			jsonError(w, "quantity must be positive", http.StatusBadRequest)
			return
		}
		requests = append(requests, service.ReserveRequest{
			OrderID:        it.OrderID,
			VariantID:      it.VariantID,
			ChannelID:      it.ChannelID,
			Quantity:       it.Quantity,
			ReservationTTL: ttl,
		})
	}

	reservations, err := h.svc.ReserveStock(r.Context(), requests)
	if err != nil {
		if errors.Is(err, service.ErrInsufficientStock) {
			jsonError(w, err.Error(), http.StatusConflict)
			return
		}
		h.log.Error("ReserveStock failed", zap.Error(err))
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, reservations)
}

// =============================================================================
// Helpers
// =============================================================================

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
