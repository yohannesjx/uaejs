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

type CategoryRepo interface {
	InsertCategory(ctx context.Context, tx pgx.Tx, c *domain.ProductCategory) error
	LinkProducts(ctx context.Context, tx pgx.Tx, categoryID uuid.UUID, productIDs []uuid.UUID) error
	DeleteMembershipsForProduct(ctx context.Context, tx pgx.Tx, productID uuid.UUID) error
	FirstCategoryIDForProduct(ctx context.Context, productID uuid.UUID) (*uuid.UUID, error)
	ListCategories(ctx context.Context, tenantID uuid.UUID) ([]*domain.ProductCategory, error)
	GetCategory(ctx context.Context, tenantID, categoryID uuid.UUID) (*domain.ProductCategory, error)
	DeleteCategory(ctx context.Context, tenantID, categoryID uuid.UUID) error
	PatchCategory(ctx context.Context, tenantID, categoryID uuid.UUID, c *domain.ProductCategory) error
	ClearMemberships(ctx context.Context, tx pgx.Tx, categoryID uuid.UUID) error
}

type CreateCategoryInput struct {
	Title       string                            `json:"title"`
	Slug        string                            `json:"slug"`
	Description *string                           `json:"description,omitempty"`
	Type        domain.CategoryType               `json:"type"`
	ImageURL    *string                           `json:"image_url,omitempty"`
	Conditions  []domain.SmartCollectionCondition `json:"conditions,omitempty"`
	ProductIDs  []uuid.UUID                       `json:"product_ids,omitempty"`
}

type CategoryService struct {
	repo CategoryRepo
	pool TxBeginner
	log  *zap.Logger
}

func NewCategoryService(repo CategoryRepo, pool TxBeginner, log *zap.Logger) *CategoryService {
	return &CategoryService{repo: repo, pool: pool, log: log}
}

func (s *CategoryService) CreateCategory(ctx context.Context, tenantID uuid.UUID, in CreateCategoryInput) (*domain.ProductCategory, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, errors.New("category title is required")
	}
	if in.Type != domain.CategoryTypeManual && in.Type != domain.CategoryTypeSmart {
		return nil, errors.New("type must be 'manual' or 'smart'")
	}
	if in.Type == domain.CategoryTypeSmart && len(in.Conditions) == 0 {
		return nil, errors.New("smart categories require at least one condition")
	}

	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		slug = strings.ToLower(strings.ReplaceAll(in.Title, " ", "-"))
	}

	cat := &domain.ProductCategory{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Title:       in.Title,
		Slug:        slug,
		Description: in.Description,
		Type:        in.Type,
		ImageURL:    in.ImageURL,
		Conditions:  in.Conditions,
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("CreateCategory begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := s.repo.InsertCategory(ctx, tx, cat); err != nil {
		return nil, fmt.Errorf("InsertCategory: %w", err)
	}

	if cat.Type == domain.CategoryTypeManual && len(in.ProductIDs) > 0 {
		if err := s.repo.LinkProducts(ctx, tx, cat.ID, in.ProductIDs); err != nil {
			return nil, fmt.Errorf("LinkProducts: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("CreateCategory commit: %w", err)
	}

	s.log.Info("created category", zap.String("id", cat.ID.String()), zap.String("type", string(cat.Type)))
	return cat, nil
}

func (s *CategoryService) ListCategories(ctx context.Context, tenantID uuid.UUID) ([]*domain.ProductCategory, error) {
	return s.repo.ListCategories(ctx, tenantID)
}

// SetProductCategory replaces all category memberships for a product with at most one category row.
func (s *CategoryService) SetProductCategory(ctx context.Context, tenantID, productID uuid.UUID, categoryID *uuid.UUID) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("SetProductCategory begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.SetProductCategoryTx(ctx, tx, tenantID, productID, categoryID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetProductCategoryTx is the transactional variant (caller commits / rolls back).
func (s *CategoryService) SetProductCategoryTx(ctx context.Context, tx pgx.Tx, tenantID, productID uuid.UUID, categoryID *uuid.UUID) error {
	if err := s.repo.DeleteMembershipsForProduct(ctx, tx, productID); err != nil {
		return fmt.Errorf("SetProductCategoryTx delete memberships: %w", err)
	}
	if categoryID == nil {
		return nil
	}
	var cTenant uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT tenant_id FROM product_categories WHERE id = $1`, *categoryID).Scan(&cTenant); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("category %s not found", categoryID)
		}
		return err
	}
	if cTenant != tenantID {
		return fmt.Errorf("category tenant mismatch")
	}
	if err := s.repo.LinkProducts(ctx, tx, *categoryID, []uuid.UUID{productID}); err != nil {
		return fmt.Errorf("SetProductCategoryTx link: %w", err)
	}
	return nil
}

// FirstCategoryIDForProduct returns one assigned category id for a product, if any.
func (s *CategoryService) FirstCategoryIDForProduct(ctx context.Context, productID uuid.UUID) (*uuid.UUID, error) {
	return s.repo.FirstCategoryIDForProduct(ctx, productID)
}

func (s *CategoryService) GetCategory(ctx context.Context, tenantID, categoryID uuid.UUID) (*domain.ProductCategory, error) {
	return s.repo.GetCategory(ctx, tenantID, categoryID)
}

func (s *CategoryService) DeleteCategory(ctx context.Context, tenantID, categoryID uuid.UUID) error {
	return s.repo.DeleteCategory(ctx, tenantID, categoryID)
}

func (s *CategoryService) PatchCategory(ctx context.Context, tenantID, categoryID uuid.UUID, updates CreateCategoryInput) (*domain.ProductCategory, error) {
	cat, err := s.repo.GetCategory(ctx, tenantID, categoryID)
	if err != nil {
		return nil, err
	}

	if updates.Title != "" {
		cat.Title = updates.Title
	}
	if updates.Slug != "" {
		cat.Slug = updates.Slug
	}
	if updates.Type != "" {
		cat.Type = updates.Type
	}
	cat.Description = updates.Description
	cat.ImageURL = updates.ImageURL

	if cat.Type == domain.CategoryTypeSmart {
		cat.Conditions = updates.Conditions
	} else {
		cat.Conditions = []domain.SmartCollectionCondition{}
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("PatchCategory begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := s.repo.PatchCategory(ctx, tenantID, categoryID, cat); err != nil {
		return nil, err
	}

	// Always clear and rebuild membership for manual patches
	if err := s.repo.ClearMemberships(ctx, tx, categoryID); err != nil {
		return nil, err
	}

	if cat.Type == domain.CategoryTypeManual && len(updates.ProductIDs) > 0 {
		if err := s.repo.LinkProducts(ctx, tx, categoryID, updates.ProductIDs); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return cat, nil
}
