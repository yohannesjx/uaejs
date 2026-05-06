package router

import (
	"encoding/json"
	"mime/multipart"
	"net/http"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type rmaHandler struct {
	rma *service.RMAService
}

func newRMAHandler(rma *service.RMAService) *rmaHandler {
	return &rmaHandler{rma: rma}
}

// POST /api/v1/returns
func (h *rmaHandler) CreateReturn(w http.ResponseWriter, r *http.Request) {
	var in service.CreateReturnInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if in.OrderID == uuid.Nil {
		http.Error(w, "order_id is required", http.StatusBadRequest)
		return
	}
	if len(in.Items) == 0 {
		http.Error(w, "at least one item is required", http.StatusBadRequest)
		return
	}

	ret, err := h.rma.CreateReturn(r.Context(), in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ret)
}

// GET /api/v1/returns/{id}
func (h *rmaHandler) GetReturn(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid return id", http.StatusBadRequest)
		return
	}

	ret, err := h.rma.GetReturnByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ret)
}

// POST /api/v1/returns/{id}/items/{itemId}/photos
// Accepts multipart/form-data with field "photo" and optional "photo_type".
func (h *rmaHandler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	returnItemID, err := uuid.Parse(chi.URLParam(r, "itemId"))
	if err != nil {
		http.Error(w, "invalid item id", http.StatusBadRequest)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32 MB max
		http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		http.Error(w, "photo field is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	photoType := r.FormValue("photo_type")
	if photoType == "" {
		photoType = "customer_submitted"
	}

	photo, qcResult, err := h.rma.UploadPhoto(r.Context(), service.UploadPhotoInput{
		ReturnItemID:  returnItemID,
		PhotoType:     toDomainPhotoType(photoType),
		Reader:        file,
		FileSizeBytes: fileSize(header),
		MIMEType:      header.Header.Get("Content-Type"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	resp := map[string]interface{}{"photo": photo}
	if qcResult != nil {
		resp["qc_result"] = qcResult
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// POST /api/v1/returns/{id}/approve
func (h *rmaHandler) ApproveReturn(w http.ResponseWriter, r *http.Request) {
	returnID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid return id", http.StatusBadRequest)
		return
	}

	var in service.ApproveInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	in.ReturnID = returnID

	ret, err := h.rma.ApproveReturn(r.Context(), in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ret)
}

// POST /api/v1/returns/{id}/reject
func (h *rmaHandler) RejectReturn(w http.ResponseWriter, r *http.Request) {
	returnID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid return id", http.StatusBadRequest)
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ret, err := h.rma.RejectReturn(r.Context(), returnID, body.Reason)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ret)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func toDomainPhotoType(s string) domain.PhotoType {
	switch s {
	case "outbound_qc":
		return domain.PhotoTypeOutboundQC
	case "warehouse_received":
		return domain.PhotoTypeWarehouseReceived
	default:
		return domain.PhotoTypeCustomerSubmitted
	}
}

func fileSize(h *multipart.FileHeader) int64 {
	if h == nil {
		return 0
	}
	return h.Size
}
