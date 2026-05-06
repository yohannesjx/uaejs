package router

import (
	"encoding/json"
	"net/http"

	"github.com/dubai-retail/os/internal/middleware"
	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type posHandler struct {
	pos *service.POSService
}

func newPOSHandler(pos *service.POSService) *posHandler {
	return &posHandler{pos: pos}
}

// POST /pos/sessions/open
func (h *posHandler) OpenSession(w http.ResponseWriter, r *http.Request) {
	var in service.OpenSessionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	// If opened_by is not in body, use the authenticated user.
	if in.OpenedBy == uuid.Nil {
		if claims := middleware.ClaimsFromContext(r.Context()); claims != nil {
			in.OpenedBy = claims.UserID
		}
	}
	session, err := h.pos.OpenSession(r.Context(), in)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(session)
}

// POST /pos/sessions/close
func (h *posHandler) CloseSession(w http.ResponseWriter, r *http.Request) {
	var in service.CloseSessionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if err := h.pos.CloseSession(r.Context(), in); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /pos/products/scan?barcode=XXX
func (h *posHandler) ScanBarcode(w http.ResponseWriter, r *http.Request) {
	barcode := r.URL.Query().Get("barcode")
	if barcode == "" {
		http.Error(w, `{"error":"barcode query param is required"}`, http.StatusBadRequest)
		return
	}
	result, err := h.pos.ScanBarcode(r.Context(), barcode)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// POST /pos/orders
func (h *posHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var in service.CreatePOSOrderInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	result, err := h.pos.CreatePOSOrder(r.Context(), in)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// POST /pos/orders/{id}/pay
func (h *posHandler) PayOrder(w http.ResponseWriter, r *http.Request) {
	orderID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid order id"}`, http.StatusBadRequest)
		return
	}

	var in service.RecordPaymentInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	in.OrderID = orderID

	receipt, err := h.pos.RecordPayment(r.Context(), in)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(receipt)
}

// GET /pos/orders/{id}/receipt
func (h *posHandler) GetReceipt(w http.ResponseWriter, r *http.Request) {
	orderID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, `{"error":"invalid order id"}`, http.StatusBadRequest)
		return
	}
	html, err := h.pos.GetReceiptHTML(r.Context(), orderID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
