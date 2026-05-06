package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

type CollectionRepo interface {
	InsertCollection(ctx context.Context, tx pgx.Tx, c *domain.ProductCollection) error
	PatchCollection(ctx context.Context, tenantID, collectionID uuid.UUID, c *domain.ProductCollection) error
	LinkProductsToCollection(ctx context.Context, tx pgx.Tx, collectionID uuid.UUID, productIDs []uuid.UUID) error
	ClearCollectionMemberships(ctx context.Context, tx pgx.Tx, collectionID uuid.UUID) error
	DeleteCollection(ctx context.Context, tenantID, collectionID uuid.UUID) error
	ListCollections(ctx context.Context, tenantID uuid.UUID) ([]*domain.ProductCollection, error)
	GetCollection(ctx context.Context, tenantID, collectionID uuid.UUID) (*domain.ProductCollection, error)
	GetCollectionBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (*domain.ProductCollection, error)
}

type UpsertCollectionInput struct {
	Title       string       `json:"title"`
	Slug        string       `json:"slug"`
	Description *string      `json:"description,omitempty"`
	ImageURL    *string      `json:"image_url,omitempty"`
	ProductIDs  *[]uuid.UUID `json:"product_ids,omitempty"`
}

type CollectionService struct {
	repo CollectionRepo
	pool TxBeginner
	log  *zap.Logger
}

func NewCollectionService(repo CollectionRepo, pool TxBeginner, log *zap.Logger) *CollectionService {
	return &CollectionService{repo: repo, pool: pool, log: log}
}

func slugifyCollectionTitle(title string, fallbackSlug string) string {
	slug := strings.TrimSpace(fallbackSlug)
	if slug != "" {
		return strings.ToLower(slug)
	}
	base := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(title, " ", "-")))
	base = strings.Trim(base, "-")
	if base == "" {
		return "collection"
	}
	return base
}

func (s *CollectionService) CreateCollection(ctx context.Context, tenantID uuid.UUID, in UpsertCollectionInput) (*domain.ProductCollection, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, errors.New("collection title is required")
	}
	slug := slugifyCollectionTitle(in.Title, in.Slug)

	col := &domain.ProductCollection{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Title:       strings.TrimSpace(in.Title),
		Slug:        slug,
		Description: in.Description,
		ImageURL:    in.ImageURL,
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("CreateCollection begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.repo.InsertCollection(ctx, tx, col); err != nil {
		return nil, fmt.Errorf("InsertCollection: %w", err)
	}
	if in.ProductIDs != nil && len(*in.ProductIDs) > 0 {
		if err := s.repo.LinkProductsToCollection(ctx, tx, col.ID, *in.ProductIDs); err != nil {
			return nil, fmt.Errorf("LinkProductsToCollection: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	s.log.Info("created collection", zap.String("id", col.ID.String()))
	return col, nil
}

func (s *CollectionService) ListCollections(ctx context.Context, tenantID uuid.UUID) ([]*domain.ProductCollection, error) {
	list, err := s.repo.ListCollections(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		return []*domain.ProductCollection{}, nil
	}
	return list, nil
}

func (s *CollectionService) GetCollection(ctx context.Context, tenantID, collectionID uuid.UUID) (*domain.ProductCollection, error) {
	return s.repo.GetCollection(ctx, tenantID, collectionID)
}

func (s *CollectionService) GetCollectionBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (*domain.ProductCollection, error) {
	return s.repo.GetCollectionBySlug(ctx, tenantID, strings.TrimSpace(slug))
}

func (s *CollectionService) PatchCollection(ctx context.Context, tenantID, collectionID uuid.UUID, updates UpsertCollectionInput) (*domain.ProductCollection, error) {
	col, err := s.repo.GetCollection(ctx, tenantID, collectionID)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(updates.Title) != "" {
		col.Title = strings.TrimSpace(updates.Title)
	}
	if s := strings.TrimSpace(updates.Slug); s != "" {
		col.Slug = strings.ToLower(s)
	}
	col.Description = updates.Description
	col.ImageURL = updates.ImageURL

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.repo.PatchCollection(ctx, tenantID, collectionID, col); err != nil {
		return nil, err
	}
	if updates.ProductIDs != nil {
		if err := s.repo.ClearCollectionMemberships(ctx, tx, collectionID); err != nil {
			return nil, err
		}
		if len(*updates.ProductIDs) > 0 {
			if err := s.repo.LinkProductsToCollection(ctx, tx, collectionID, *updates.ProductIDs); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.repo.GetCollection(ctx, tenantID, collectionID)
}

func (s *CollectionService) DeleteCollection(ctx context.Context, tenantID, collectionID uuid.UUID) error {
	return s.repo.DeleteCollection(ctx, tenantID, collectionID)
}
