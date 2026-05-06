package router

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/dubai-retail/os/internal/middleware"
	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type customerHandler struct {
	customerSvc *service.CustomerService
	loyaltySvc  *service.LoyaltyService
}

func newCustomerHandler(cs *service.CustomerService, ls *service.LoyaltyService) *customerHandler {
	return &customerHandler{customerSvc: cs, loyaltySvc: ls}
}

// POST /customers
func (h *customerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in service.CreateCustomerInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if in.TenantID == uuid.Nil {
		in.TenantID = middleware.TenantFromContext(r.Context())
	}
	customer, err := h.customerSvc.Create(r.Context(), in)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(customer)
}

// GET /customers/{id}
func (h *customerHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid customer id"}`, http.StatusBadRequest)
		return
	}
	customer, err := h.customerSvc.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"customer not found"}`, http.StatusNotFound)
		return
	}
	// Attach balance.
	acc, _ := h.loyaltySvc.GetBalance(r.Context(), id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"customer":        customer,
		"loyalty_account": acc,
	})
}

// POST /customers/{id}/points/add
func (h *customerHandler) AddPoints(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid customer id"}`, http.StatusBadRequest)
		return
	}
	var in service.AwardPointsInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	in.CustomerID = id
	tx, err := h.loyaltySvc.AwardPoints(r.Context(), in)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tx)
}

// POST /customers/{id}/points/redeem
func (h *customerHandler) RedeemPoints(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid customer id"}`, http.StatusBadRequest)
		return
	}
	var in service.RedeemInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	in.CustomerID = id
	result, err := h.loyaltySvc.RedeemPoints(r.Context(), in)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GET /customers/{id}/points/history
func (h *customerHandler) PointsHistory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid customer id"}`, http.StatusBadRequest)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	txs, err := h.loyaltySvc.GetHistory(r.Context(), id, limit)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(txs)
}
