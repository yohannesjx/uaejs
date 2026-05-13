package router

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	appMiddleware "github.com/dubai-retail/os/internal/middleware"
	"github.com/dubai-retail/os/internal/service"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type mediaHandler struct {
	svc *service.MediaService
	log *zap.Logger
}

func newMediaHandler(svc *service.MediaService, log *zap.Logger) *mediaHandler {
	return &mediaHandler{svc: svc, log: log}
}

// UploadMedia handles POST /admin/media/upload (multipart/form-data)
func (h *mediaHandler) UploadMedia(w http.ResponseWriter, r *http.Request) {
	tenantID := appMiddleware.TenantFromContext(r.Context())

	// Max 50MB per file
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	asset, err := h.svc.UploadMedia(r.Context(), service.UploadMediaInput{
		TenantID:  tenantID,
		Filename:  header.Filename,
		MimeType:  header.Header.Get("Content-Type"),
		SizeBytes: header.Size,
		File:      file,
	})
	if err != nil {
		h.log.Error("failed to upload media", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(asset)
}

// ImportMediaFromURL handles POST /admin/media/import-url (JSON { "url": "https://..." }).
func (h *mediaHandler) ImportMediaFromURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenantID := appMiddleware.TenantFromContext(r.Context())

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}

	asset, err := h.svc.ImportMediaFromURL(r.Context(), tenantID, req.URL)
	if err != nil {
		h.log.Warn("import media from url failed", zap.Error(err), zap.String("url", req.URL))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(asset)
}

// ListMedia handles GET /admin/media
func (h *mediaHandler) ListMedia(w http.ResponseWriter, r *http.Request) {
	tenantID := appMiddleware.TenantFromContext(r.Context())

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	filter := domain.MediaFilter{
		TenantID: tenantID,
		Limit:    limit,
		Type:     r.URL.Query().Get("type"),
		Search:   r.URL.Query().Get("search"),
	}

	if c := r.URL.Query().Get("cursor"); c != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, c); err == nil {
			filter.Cursor = &parsed
		}
	}

	page, err := h.svc.ListMedia(r.Context(), filter)
	if err != nil {
		h.log.Error("failed to list media", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}

// PatchMedia handles PATCH /admin/media/{id}
func (h *mediaHandler) PatchMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	var req struct {
		Alt  *string  `json:"alt"`
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.svc.PatchMedia(r.Context(), id, req.Alt, req.Tags); err != nil {
		h.log.Error("failed to patch media", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// DeleteMedia handles DELETE /admin/media/{id}
func (h *mediaHandler) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if err := h.svc.DeleteMedia(r.Context(), id); err != nil {
		h.log.Error("failed to delete media", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
