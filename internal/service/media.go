package service

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
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
