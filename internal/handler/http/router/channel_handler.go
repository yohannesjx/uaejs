package router

import (
	"encoding/json"
	"net/http"

	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type channelHandler struct {
	svc *service.ChannelSyncService
}

func newChannelHandler(svc *service.ChannelSyncService) *channelHandler {
	return &channelHandler{svc: svc}
}

// GET /admin/channels
func (h *channelHandler) ListPlatforms(w http.ResponseWriter, r *http.Request) {
	platforms, err := h.svc.ListPlatforms(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(platforms)
}

// POST /admin/channels/connect
func (h *channelHandler) ConnectPlatform(w http.ResponseWriter, r *http.Request) {
	var in service.ConnectPlatformInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if in.APIKey == "" || in.Type == "" {
		http.Error(w, `{"error":"type and api_key are required"}`, http.StatusBadRequest)
		return
	}
	account, err := h.svc.ConnectPlatform(r.Context(), in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(account)
}

// POST /admin/channels/{id}/sync-products
func (h *channelHandler) SyncProducts(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid account id"}`, http.StatusBadRequest)
		return
	}
	// No body – sync all already-mapped products
	if err := h.svc.SyncProducts(r.Context(), accountID, nil, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"message":"product sync enqueued"}`))
}

// POST /admin/channels/{id}/sync-inventory
func (h *channelHandler) SyncInventory(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid account id"}`, http.StatusBadRequest)
		return
	}
	if err := h.svc.SyncInventory(r.Context(), accountID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"message":"inventory sync complete"}`))
}

// GET /admin/channels/orders
func (h *channelHandler) ListPlatformOrders(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	orders, err := h.svc.ListPlatformOrders(r.Context(), status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}
