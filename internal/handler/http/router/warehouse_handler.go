package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/middleware"
	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type warehouseHandler struct {
	svc         *service.WarehouseService
	activityLog *service.ActivityLogService
}

type transferItemRequest struct {
	ID        *uuid.UUID `json:"id,omitempty"`
	VariantID uuid.UUID  `json:"variant_id"`
	Quantity  int        `json:"quantity"`
}

type transferRequest struct {
	Reference              string                `json:"reference"`
	OriginWarehouseID      uuid.UUID             `json:"origin_warehouse_id"`
	DestinationWarehouseID uuid.UUID             `json:"destination_warehouse_id"`
	Notes                  *string               `json:"notes"`
	Tags                   []string              `json:"tags"`
	Items                  []transferItemRequest `json:"items"`
	Status                 string                `json:"status,omitempty"`
}

func newWarehouseHandler(svc *service.WarehouseService, activityLog *service.ActivityLogService) *warehouseHandler {
	return &warehouseHandler{svc: svc, activityLog: activityLog}
}

func (h *warehouseHandler) recordActivity(r *http.Request, eventType, title, description, subjectID, subjectType string, metadata map[string]any) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil || h.activityLog == nil {
		return
	}
	_ = h.activityLog.Record(r.Context(), service.RecordInput{
		TenantID:    middleware.TenantFromContext(r.Context()),
		ActorID:     claims.UserID,
		ActorEmail:  claims.Email,
		EventType:   eventType,
		Title:       title,
		Description: description,
		SubjectID:   subjectID,
		SubjectType: subjectType,
		Metadata:    metadata,
	})
}

// POST /admin/warehouses
func (h *warehouseHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in service.CreateWarehouseInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if in.TenantID == uuid.Nil {
		in.TenantID = middleware.TenantFromContext(r.Context())
	}
	warehouse, err := h.svc.CreateWarehouse(r.Context(), in)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	h.recordActivity(r, "warehouse.created", "Created warehouse", warehouse.Name, warehouse.ID.String(), "warehouse", nil)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(warehouse)
}

// PUT /admin/warehouses/{id}
func (h *warehouseHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid warehouse id"}`, http.StatusBadRequest)
		return
	}
	var wh domain.Warehouse
	if err := json.NewDecoder(r.Body).Decode(&wh); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	wh.ID = id
	wh.TenantID = middleware.TenantFromContext(r.Context())
	if err := h.svc.UpdateWarehouse(r.Context(), &wh); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /admin/warehouses
func (h *warehouseHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if _, err := h.svc.EnsureDefaultWarehouse(r.Context(), tenantID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	warehouses, err := h.svc.ListWarehouses(r.Context(), tenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(warehouses)
}

// GET /admin/inventory
func (h *warehouseHandler) ListInventoryRows(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	var warehouseID *uuid.UUID
	if wID := strings.TrimSpace(r.URL.Query().Get("warehouse_id")); wID != "" {
		parsed, err := uuid.Parse(wID)
		if err != nil {
			http.Error(w, `{"error":"invalid warehouse id"}`, http.StatusBadRequest)
			return
		}
		warehouseID = &parsed
	}
	product := strings.TrimSpace(r.URL.Query().Get("product"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	lowStockOnly := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("low_stock")), "true")

	items, err := h.svc.ListInventoryRows(r.Context(), tenantID, warehouseID, product, category, lowStockOnly)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"items": items,
		"total": len(items),
	})
}

// GET /admin/warehouses/{id}/inventory
func (h *warehouseHandler) GetInventory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid warehouse id"}`, http.StatusBadRequest)
		return
	}
	stocks, err := h.svc.GetInventory(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stocks)
}

// POST /admin/warehouses/transfer
func (h *warehouseHandler) Transfer(w http.ResponseWriter, r *http.Request) {
	var req domain.TransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	result, err := h.svc.Transfer(r.Context(), req)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	h.recordActivity(r, "warehouse.transfer", "Warehouse stock transfer",
		fmt.Sprintf("Transferred %d units of variant %s from %s to %s", req.Quantity, req.VariantID, req.FromWarehouseID, req.ToWarehouseID),
		req.VariantID.String(), "transfer",
		map[string]any{"from_warehouse_id": req.FromWarehouseID.String(), "to_warehouse_id": req.ToWarehouseID.String(), "quantity": req.Quantity})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// POST /admin/warehouses/{id}/stock
func (h *warehouseHandler) SetStock(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid warehouse id"}`, http.StatusBadRequest)
		return
	}
	var in service.SetStockInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	in.WarehouseID = id
	if err := h.svc.SetStock(r.Context(), in); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /admin/inventory/adjust
func (h *warehouseHandler) AdjustStock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WarehouseID    uuid.UUID `json:"warehouse_id"`
		VariantID      uuid.UUID `json:"variant_id"`
		AdjustmentType string    `json:"adjustment_type"`
		Quantity       int       `json:"quantity"`
		Reason         *string   `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	stock, err := h.svc.AdjustInventory(r.Context(), service.AdjustInventoryInput{
		WarehouseID:    req.WarehouseID,
		VariantID:      req.VariantID,
		AdjustmentType: domain.InventoryAdjustmentType(req.AdjustmentType),
		Quantity:       req.Quantity,
		Reason:         req.Reason,
	})
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stock)
}

// GET /admin/transfers
func (h *warehouseHandler) ListTransfers(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	items, err := h.svc.ListTransfers(r.Context(), tenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"items": items, "total": len(items)})
}

// GET /admin/transfers/{id}
func (h *warehouseHandler) GetTransfer(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid transfer id"}`, http.StatusBadRequest)
		return
	}
	item, err := h.svc.GetTransfer(r.Context(), tenantID, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

// POST /admin/transfers
func (h *warehouseHandler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	var req transferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	items := make([]domain.TransferItem, 0, len(req.Items))
	for _, it := range req.Items {
		id := uuid.Nil
		if it.ID != nil {
			id = *it.ID
		}
		items = append(items, domain.TransferItem{
			ID:        id,
			VariantID: it.VariantID,
			Quantity:  it.Quantity,
		})
	}
	status := domain.TransferStatus(req.Status)
	if status == "" {
		status = domain.TransferStatusDraft
	}
	created, err := h.svc.CreateTransfer(r.Context(), service.CreateTransferInput{
		TenantID:               tenantID,
		Reference:              req.Reference,
		OriginWarehouseID:      req.OriginWarehouseID,
		DestinationWarehouseID: req.DestinationWarehouseID,
		Notes:                  req.Notes,
		Tags:                   req.Tags,
		Items:                  items,
		Status:                 status,
	})
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// PATCH /admin/transfers/{id}
func (h *warehouseHandler) UpdateTransfer(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid transfer id"}`, http.StatusBadRequest)
		return
	}
	var req transferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	items := make([]domain.TransferItem, 0, len(req.Items))
	for _, it := range req.Items {
		itemID := uuid.Nil
		if it.ID != nil {
			itemID = *it.ID
		}
		items = append(items, domain.TransferItem{ID: itemID, VariantID: it.VariantID, Quantity: it.Quantity})
	}
	updated, err := h.svc.UpdateTransfer(r.Context(), tenantID, id, service.CreateTransferInput{
		TenantID:               tenantID,
		Reference:              req.Reference,
		OriginWarehouseID:      req.OriginWarehouseID,
		DestinationWarehouseID: req.DestinationWarehouseID,
		Notes:                  req.Notes,
		Tags:                   req.Tags,
		Items:                  items,
	})
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// POST /admin/transfers/{id}/status
func (h *warehouseHandler) TransitionTransfer(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid transfer id"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	updated, err := h.svc.TransitionTransfer(r.Context(), tenantID, id, domain.TransferStatus(req.Status))
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}
