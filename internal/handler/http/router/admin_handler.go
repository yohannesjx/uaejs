package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// adminHandler exposes privileged operations behind /admin routes.
// In production these routes must be gated by an API key / mTLS middleware.
type adminHandler struct {
	batchImport *service.BatchImportService
	aspSandbox  *service.ASPSandboxService
}

func newAdminHandler(batchImport *service.BatchImportService, asp *service.ASPSandboxService) *adminHandler {
	return &adminHandler{batchImport: batchImport, aspSandbox: asp}
}

// POST /admin/batches/import
//
// Accepts multipart/form-data with:
//
//	field "file" – CSV or JSON file upload
//
// Or application/json body with an array of ImportRow objects.
func (h *adminHandler) ImportBatch(w http.ResponseWriter, r *http.Request) {
	importedBy := r.Header.Get("X-Admin-User")
	if importedBy == "" {
		importedBy = "api"
	}

	contentType := r.Header.Get("Content-Type")
	var result *service.BatchImportResult
	var err error

	switch {
	case strings.Contains(contentType, "multipart/form-data"):
		if err = r.ParseMultipartForm(64 << 20); err != nil { // 64 MB
			http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, header, ferr := r.FormFile("file")
		if ferr != nil {
			http.Error(w, "field 'file' is required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(header.Filename))
		switch ext {
		case ".csv":
			result, err = h.batchImport.ImportFromCSV(r.Context(), header.Filename, importedBy, file)
		case ".json":
			result, err = h.batchImport.ImportFromJSON(r.Context(), header.Filename, importedBy, file)
		default:
			http.Error(w, fmt.Sprintf("unsupported file type %q – use .csv or .json", ext), http.StatusBadRequest)
			return
		}

	case strings.Contains(contentType, "application/json"):
		result, err = h.batchImport.ImportFromJSON(r.Context(), "api_import.json", importedBy, r.Body)

	default:
		http.Error(w, "Content-Type must be multipart/form-data or application/json", http.StatusUnsupportedMediaType)
		return
	}

	if err != nil {
		http.Error(w, "import failed: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	status := http.StatusCreated
	if result.FailedRows > 0 && result.ImportedRows == 0 {
		status = http.StatusUnprocessableEntity
	} else if result.FailedRows > 0 {
		status = http.StatusMultiStatus // 207 – partial success
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(result)
}

// POST /admin/invoices/:orderId/sandbox
// Submits the UBL XML for an order to the ASP sandbox for validation.
func (h *adminHandler) SubmitToSandbox(w http.ResponseWriter, r *http.Request) {
	orderID, err := uuid.Parse(chi.URLParam(r, "orderId"))
	if err != nil {
		http.Error(w, "invalid order id", http.StatusBadRequest)
		return
	}

	result, err := h.aspSandbox.Submit(r.Context(), orderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
