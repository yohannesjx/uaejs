package router

import (
	"encoding/json"
	"net/http"

	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type shippingHandler struct {
	svc *service.ShippingService
}

func newShippingHandler(svc *service.ShippingService) *shippingHandler {
	return &shippingHandler{svc: svc}
}

// POST /admin/shipping/accounts
func (h *shippingHandler) AddAccount(w http.ResponseWriter, r *http.Request) {
	var in service.AddAccountInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	acc, err := h.svc.AddAccount(r.Context(), in)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(acc)
}

// POST /admin/shipments/create
func (h *shippingHandler) CreateShipment(w http.ResponseWriter, r *http.Request) {
	var in service.CreateShipmentInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	shipment, err := h.svc.CreateShipment(r.Context(), in)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(shipment)
}

// GET /admin/shipments/{id}
func (h *shippingHandler) GetShipment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid shipment id"}`, http.StatusBadRequest)
		return
	}
	shipment, err := h.svc.GetShipment(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(shipment)
}

// GET /admin/shipments/{id}/tracking
func (h *shippingHandler) GetTracking(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid shipment id"}`, http.StatusBadRequest)
		return
	}
	if err := h.svc.SyncTracking(r.Context(), id); err != nil {
		// Sync errors are non-fatal; return stale data if available.
	}
	shipment, err := h.svc.GetShipment(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(shipment.Events)
}
