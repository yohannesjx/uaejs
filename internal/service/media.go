package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/repository/postgres"
	"github.com/dubai-retail/os/internal/storage"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type MediaService struct {
	repo  *postgres.MediaRepository
	store storage.ObjectStore
	log   *zap.Logger
}

func NewMediaService(repo *postgres.MediaRepository, store storage.ObjectStore, log *zap.Logger) *MediaService {
	return &MediaService{
		repo:  repo,
		store: store,
		log:   log,
	}
}

type UploadMediaInput struct {
	TenantID  uuid.UUID
	Filename  string
	MimeType  string
	SizeBytes int64
	File      io.Reader
}

func (s *MediaService) UploadMedia(ctx context.Context, in UploadMediaInput) (*domain.MediaAsset, error) {
	assetID := uuid.New()
	ext := filepath.Ext(in.Filename)

	var url string
	if s.store != nil {
		var err error
		url, err = s.store.Upload(ctx, in.TenantID.String(), assetID.String(), ext, in.File)
		if err != nil {
			return nil, fmt.Errorf("upload to store: %w", err)
		}
	} else {
		// Mock URL if no store is configured
		url = fmt.Sprintf("/uploads/mock_%s%s", assetID.String(), ext)
	}

	asset := domain.MediaAsset{
		ID:        assetID,
		TenantID:  in.TenantID,
		URL:       url,
		MimeType:  in.MimeType,
		SizeBytes: in.SizeBytes,
		Alt:       &in.Filename,
		Tags:      []string{},
		SortOrder: 0,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.repo.InsertMedia(ctx, asset); err != nil {
		return nil, fmt.Errorf("repo.InsertMedia: %w", err)
	}

	return &asset, nil
}

const maxImportMediaBytes = 52 << 20 // slightly above multipart limit

var importHTTPClient = &http.Client{Timeout: 45 * time.Second}

// ImportMediaFromURL downloads an image over HTTP(S) and stores it like a normal upload.
func (s *MediaService) ImportMediaFromURL(ctx context.Context, tenantID uuid.UUID, rawURL string) (*domain.MediaAsset, error) {
	rawURL = strings.TrimSpace(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("only http(s) URLs are allowed")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "metadata.google.internal" || strings.HasPrefix(host, "127.") {
		return nil, fmt.Errorf("url host is not allowed")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "DubaiRetailOS-MediaImport/1.0")

	resp, err := importHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch url: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("remote returned status %d", resp.StatusCode)
	}

	mt := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if mt == "" {
		mt = "application/octet-stream"
	}
	if !strings.HasPrefix(mt, "image/") && mt != "application/octet-stream" {
		return nil, fmt.Errorf("unsupported content type %q (expected image/*)", mt)
	}

	filename := path.Base(u.Path)
	if filename == "" || filename == "/" || filename == "." {
		filename = "import"
	}
	if filepath.Ext(filename) == "" {
		switch {
		case strings.Contains(mt, "jpeg"), strings.Contains(mt, "jpg"):
			filename += ".jpg"
		case strings.Contains(mt, "png"):
			filename += ".png"
		case strings.Contains(mt, "webp"):
			filename += ".webp"
		case strings.Contains(mt, "gif"):
			filename += ".gif"
		default:
			filename += ".bin"
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImportMediaBytes))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("empty response body")
	}

	return s.UploadMedia(ctx, UploadMediaInput{
		TenantID:  tenantID,
		Filename:  filename,
		MimeType:  mt,
		SizeBytes: int64(len(body)),
		File:      bytes.NewReader(body),
	})
}

func (s *MediaService) ListMedia(ctx context.Context, filter domain.MediaFilter) (domain.MediaPage, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	return s.repo.ListMedia(ctx, filter)
}

func (s *MediaService) PatchMedia(ctx context.Context, id string, alt *string, tags []string) error {
	return s.repo.PatchMedia(ctx, id, alt, tags)
}

func (s *MediaService) DeleteMedia(ctx context.Context, id string) error {
	asset, err := s.repo.GetMedia(ctx, id)
	if err != nil {
		return err
	}
	if asset == nil {
		return nil // Already deleted or not found
	}

	// Delete from DB first
	if err := s.repo.DeleteMedia(ctx, id); err != nil {
		return fmt.Errorf("repo.DeleteMedia: %w", err)
	}

	if s.store != nil {
		// Best effort delete from store
		if err := s.store.Delete(ctx, asset.URL); err != nil {
			s.log.Error("failed to delete media from store", zap.String("url", asset.URL), zap.Error(err))
		}
	}

	return nil
}
