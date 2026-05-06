package router

import (
	"encoding/json"
	"net/http"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type supplierHandler struct {
	svc *service.SupplierService
}

func newSupplierHandler(svc *service.SupplierService) *supplierHandler {
	return &supplierHandler{svc: svc}
}

// GET /admin/suppliers
func (h *supplierHandler) ListSuppliers(w http.ResponseWriter, r *http.Request) {
	suppliers, err := h.svc.ListSuppliers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(suppliers)
}

// POST /admin/suppliers
func (h *supplierHandler) CreateSupplier(w http.ResponseWriter, r *http.Request) {
	var s domain.Supplier
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if s.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	created, err := h.svc.CreateSupplier(r.Context(), s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// PATCH /admin/suppliers/{id}
func (h *supplierHandler) UpdateSupplier(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid supplier id"}`, http.StatusBadRequest)
		return
	}
	var s domain.Supplier
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	s.ID = id
	updated, err := h.svc.UpdateSupplier(r.Context(), s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// ── Purchase Orders ───────────────────────────────────────────────────────────

// GET /admin/purchase-orders
func (h *supplierHandler) ListPOs(w http.ResponseWriter, r *http.Request) {
	pos, err := h.svc.ListPurchaseOrders(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pos)
}

// POST /admin/purchase-orders
func (h *supplierHandler) CreatePO(w http.ResponseWriter, r *http.Request) {
	var in service.CreatePOInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	po, err := h.svc.CreatePurchaseOrder(r.Context(), in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(po)
}

// POST /admin/purchase-orders/{id}/items
func (h *supplierHandler) AddPOItem(w http.ResponseWriter, r *http.Request) {
	poID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid purchase order id"}`, http.StatusBadRequest)
		return
	}
	var in service.AddPOItemInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	in.PurchaseOrderID = poID
	item, err := h.svc.AddPOItem(r.Context(), in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(item)
}

// POST /admin/purchase-orders/{id}/receive
func (h *supplierHandler) ReceivePO(w http.ResponseWriter, r *http.Request) {
	poID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid purchase order id"}`, http.StatusBadRequest)
		return
	}
	var in service.ReceiveInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	in.PurchaseOrderID = poID
	batch, err := h.svc.ReceivePurchaseOrder(r.Context(), in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":  "purchase order received",
		"batch_id": batch.ID,
	})
}
