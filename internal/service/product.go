package service

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Repository interfaces
// =============================================================================

type ProductRepo interface {
	InsertProduct(ctx context.Context, tx pgx.Tx, p *domain.Product) error
	InsertDraftProduct(ctx context.Context, tx pgx.Tx) (uuid.UUID, error)
	PatchProduct(ctx context.Context, tx pgx.Tx, id uuid.UUID, updates map[string]any) error
	ResolveUniqueProductSlug(ctx context.Context, tx pgx.Tx, base string, excludeID *uuid.UUID) (string, error)
	InsertVariant(ctx context.Context, tx pgx.Tx, v *domain.Variant) error
	InitInventory(ctx context.Context, tx pgx.Tx, variantID uuid.UUID) error
	GetProductByID(ctx context.Context, id uuid.UUID) (*domain.Product, error)
	GetVariantBySKU(ctx context.Context, sku string) (*domain.Variant, error)
	GetVariantByID(ctx context.Context, id uuid.UUID) (*domain.Variant, error)
	ListVariantsByProduct(ctx context.Context, productID uuid.UUID) ([]*domain.Variant, error)
	UpdateVariant(ctx context.Context, tx pgx.Tx, v *domain.Variant) error
	ReplaceVariantMedia(ctx context.Context, tx pgx.Tx, variantID uuid.UUID, urls []string) error
	DeleteVariant(ctx context.Context, tx pgx.Tx, id uuid.UUID) error
	UpsertChannelPrice(ctx context.Context, cp *domain.ChannelPrice) error
	GetChannelPrice(ctx context.Context, variantID, channelID uuid.UUID) (*domain.ChannelPrice, error)
	GetChannelByType(ctx context.Context, channelType domain.ChannelType) (*domain.Channel, error)
}

// ProductCollectionMembershipLinker syncs storefront collections independently from textual categories.
type ProductCollectionMembershipLinker interface {
	ListCollectionIDsForProduct(ctx context.Context, productID uuid.UUID) ([]uuid.UUID, error)
	ReplaceMembershipsForProduct(ctx context.Context, productID uuid.UUID, collectionIDs []uuid.UUID) error
}

// =============================================================================
// Input / output DTOs
// =============================================================================

// VariantInput is the payload for a single variant in CreateProductWithVariants.
// SKU is ignored on create: the server assigns a unique JS-######## SKU per variant.
type VariantInput struct {
	SKU       string   `json:"sku,omitempty"`
	Barcode   *string  `json:"barcode,omitempty"`
	Color     *string  `json:"color,omitempty"`
	Size      *string  `json:"size,omitempty"`
	WeightG   *float64 `json:"weight_g,omitempty"`
	ImageURL  *string  `json:"image_url,omitempty"`
	Price     *string  `json:"price,omitempty"`
	SalePrice *string  `json:"sale_price,omitempty"`
	Quantity  *int     `json:"quantity,omitempty"`
	MediaURLs []string `json:"media_urls,omitempty"`
	Cost      *string  `json:"cost,omitempty"`
}

// productCategorySync assigns products to product_categories via memberships.
type productCategorySync interface {
	SetProductCategory(ctx context.Context, tenantID, productID uuid.UUID, categoryID *uuid.UUID) error
	SetProductCategoryTx(ctx context.Context, tx pgx.Tx, tenantID, productID uuid.UUID, categoryID *uuid.UUID) error
	SetProductCategoriesTx(ctx context.Context, tx pgx.Tx, tenantID, productID uuid.UUID, categoryIDs []uuid.UUID) error
	FirstCategoryIDForProduct(ctx context.Context, productID uuid.UUID) (*uuid.UUID, error)
}

// CreateProductInput is the request payload for the CreateProductWithVariants service method.
type CreateProductInput struct {
	TenantID        uuid.UUID             `json:"tenant_id,omitempty"`
	Name            string                `json:"name"`
	Slug            *string               `json:"slug,omitempty"`
	NameAR          *string               `json:"name_ar,omitempty"`
	Description     *string               `json:"description,omitempty"`
	Brand           *string               `json:"brand,omitempty"`
	Category        *string               `json:"category,omitempty"`
	SubCategory     *string               `json:"sub_category,omitempty"`
	CategoryID      *uuid.UUID            `json:"category_id,omitempty"`
	CategoryIDs     []uuid.UUID           `json:"category_ids,omitempty"`
	VATType         domain.VATType        `json:"vat_type"`
	HSCode          *string               `json:"hs_code,omitempty"`
	CountryOfOrigin string                `json:"country_of_origin"`
	TrackInventory  bool                  `json:"track_inventory"`
	WarehouseID     *uuid.UUID            `json:"warehouse_id,omitempty"`
	Status          *domain.ProductStatus `json:"status,omitempty"`
	Variants        []VariantInput        `json:"variants"`
}

// UpdateProductInput is the request payload for partial updates (PATCH) during draft autho-saves.
type UpdateProductInput struct {
	Name *string `json:"name"`
	// Title aliases name for admin dashboard JSON (theme uses "title" on products).
	Title           *string               `json:"title,omitempty"`
	Slug            *string               `json:"slug"`
	NameAR          *string               `json:"name_ar"`
	Description     *string               `json:"description"`
	Brand           *string               `json:"brand"`
	Category        *string               `json:"category"`
	SubCategory     *string               `json:"sub_category"`
	Status          *domain.ProductStatus `json:"status"`
	VATType         *domain.VATType       `json:"vat_type"`
	HSCode          *string               `json:"hs_code"`
	CountryOfOrigin *string               `json:"country_of_origin"`
	// When non-nil, replaces all collection memberships for this product ([] clears).
	CollectionIDs *[]uuid.UUID `json:"collection_ids,omitempty"`
	// When non-nil, syncs category membership. Use "" to clear; omit or null to leave unchanged.
	CategoryID *string `json:"category_id,omitempty"`
	// CallerTenantID is set by HTTP handlers for category membership sync (not read from JSON).
	CallerTenantID uuid.UUID `json:"-"`
}

type UpsertVariantInput struct {
	SKU       string   `json:"sku"`
	Color     *string  `json:"color,omitempty"`
	Size      *string  `json:"size,omitempty"`
	ImageURL  *string  `json:"image_url,omitempty"`
	Price     *string  `json:"price,omitempty"`
	SalePrice *string  `json:"sale_price,omitempty"`
	Quantity  *int     `json:"quantity,omitempty"`
	MediaURLs []string `json:"media_urls,omitempty"`
	Cost      *string  `json:"cost,omitempty"`
}

// CreateProductResult is returned after a successful product creation.
type CreateProductResult struct {
	Product       *domain.Product   `json:"product"`
	Variants      []*domain.Variant `json:"variants"`
	CollectionIDs []uuid.UUID       `json:"collection_ids,omitempty"`
	CategoryID    *uuid.UUID        `json:"category_id,omitempty"`
}

// SetPriceInput sets or replaces a channel price.
type SetPriceInput struct {
	VariantID      uuid.UUID       `json:"variant_id"`
	ChannelID      uuid.UUID       `json:"channel_id"`
	Price          decimal.Decimal `json:"price"`
	Currency       string          `json:"currency"`
	EffectiveUntil *time.Time      `json:"effective_until,omitempty"`
}

// =============================================================================
// Errors
// =============================================================================

var (
	ErrDuplicateSKU    = errors.New("SKU already exists")
	ErrProductNotFound = errors.New("product not found")
)

// =============================================================================
// ProductService
// =============================================================================

type ProductService struct {
	repo ProductRepo
	pool TxBeginner
	log  *zap.Logger
	pcm  ProductCollectionMembershipLinker
	pcs  productCategorySync
}

func NewProductService(repo ProductRepo, pool TxBeginner, log *zap.Logger, pcm ProductCollectionMembershipLinker, pcs productCategorySync) *ProductService {
	return &ProductService{repo: repo, pool: pool, log: log, pcm: pcm, pcs: pcs}
}

// CreateDraft initializes an empty product in the 'draft' status.
func (s *ProductService) CreateDraft(ctx context.Context) (*domain.Product, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("CreateDraft begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("CreateDraft rollback", zap.Error(rbErr))
		}
	}()

	id, err := s.repo.InsertDraftProduct(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("CreateDraft insert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("CreateDraft commit: %w", err)
	}

	return s.repo.GetProductByID(ctx, id)
}

// UpdateProduct applies partial changes to an existing product gracefully.
func (s *ProductService) UpdateProduct(ctx context.Context, id uuid.UUID, input UpdateProductInput) error {
	if input.Title != nil && input.Name == nil {
		name := strings.TrimSpace(*input.Title)
		input.Name = &name
	}

	updates := make(map[string]any)

	if input.Name != nil {
		updates["name"] = *input.Name
	}
	if input.NameAR != nil {
		updates["name_ar"] = *input.NameAR
	}
	if input.Description != nil {
		updates["description"] = *input.Description
	}
	if input.Brand != nil {
		updates["brand"] = *input.Brand
	}
	if input.Category != nil {
		updates["category"] = *input.Category
	}
	if input.SubCategory != nil {
		updates["sub_category"] = *input.SubCategory
	}
	if input.Status != nil {
		updates["status"] = *input.Status
	}
	if input.VATType != nil {
		updates["vat_type"] = *input.VATType
	}
	if input.HSCode != nil {
		updates["hs_code"] = *input.HSCode
	}
	if input.CountryOfOrigin != nil {
		updates["country_of_origin"] = *input.CountryOfOrigin
	}

	collectionSync := input.CollectionIDs != nil && s.pcm != nil
	categorySync := input.CategoryID != nil && s.pcs != nil
	if len(updates) == 0 && !collectionSync && !categorySync {
		return nil // Nothing to patch
	}

	if len(updates) > 0 {
		tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("UpdateProduct begin tx: %w", err)
		}
		defer func() {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				s.log.Error("UpdateProduct rollback", zap.Error(rbErr))
			}
		}()

		var slugSource string
		switch {
		case input.Slug != nil:
			slugSource = *input.Slug
		case input.Name != nil:
			slugSource = *input.Name
		}
		if strings.TrimSpace(slugSource) != "" {
			base := slugify(slugSource)
			if base != "" {
				uniqueSlug, err := s.repo.ResolveUniqueProductSlug(ctx, tx, base, &id)
				if err != nil {
					return fmt.Errorf("UpdateProduct resolve slug: %w", err)
				}
				updates["slug"] = uniqueSlug
			}
		}

		if err := s.repo.PatchProduct(ctx, tx, id, updates); err != nil {
			return fmt.Errorf("UpdateProduct patch: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("UpdateProduct commit: %w", err)
		}
		s.log.Info("product updated via patch", zap.String("product_id", id.String()))
	}

	if collectionSync {
		if err := s.pcm.ReplaceMembershipsForProduct(ctx, id, *input.CollectionIDs); err != nil {
			return fmt.Errorf("UpdateProduct collections: %w", err)
		}
		s.log.Info("product collections synced", zap.String("product_id", id.String()))
	}

	if categorySync {
		tenantID := input.CallerTenantID
		if tenantID == uuid.Nil {
			tenantID = domain.DefaultTenantID
		}
		var catID *uuid.UUID
		if raw := strings.TrimSpace(*input.CategoryID); raw != "" {
			parsed, err := uuid.Parse(raw)
			if err != nil {
				return fmt.Errorf("UpdateProduct: invalid category_id: %w", err)
			}
			catID = &parsed
		}
		if err := s.pcs.SetProductCategory(ctx, tenantID, id, catID); err != nil {
			return fmt.Errorf("UpdateProduct category: %w", err)
		}
		s.log.Info("product category synced", zap.String("product_id", id.String()))
	}

	return nil
}

// CreateProductWithVariants atomically creates a product, all its variants,
// and seeds a zero-stock inventory row for each variant in a single transaction.
func (s *ProductService) CreateProductWithVariants(
	ctx context.Context,
	input CreateProductInput,
) (*CreateProductResult, error) {
	if err := validateCreateProductInput(input); err != nil {
		return nil, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return nil, fmt.Errorf("CreateProductWithVariants: begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("CreateProductWithVariants: rollback", zap.Error(rbErr))
		}
	}()

	// -------------------------------------------------------------------------
	// 1. Create the parent product.
	// -------------------------------------------------------------------------
	nameStr := strings.TrimSpace(input.Name)
	slugSource := nameStr
	if input.Slug != nil && strings.TrimSpace(*input.Slug) != "" {
		slugSource = *input.Slug
	}
	baseSlug := slugify(slugSource)
	uniqueSlug, err := s.repo.ResolveUniqueProductSlug(ctx, tx, baseSlug, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateProductWithVariants: resolve slug: %w", err)
	}

	status := domain.StatusActive
	if input.Status != nil {
		switch *input.Status {
		case domain.StatusDraft, domain.StatusActive, domain.StatusArchived:
			status = *input.Status
		}
	}

	product := &domain.Product{
		ID:              uuid.New(),
		Name:            &nameStr,
		Slug:            &uniqueSlug,
		NameAR:          input.NameAR,
		Description:     input.Description,
		Brand:           input.Brand,
		Category:        input.Category,
		SubCategory:     input.SubCategory,
		Status:          status,
		VATType:         input.VATType,
		HSCode:          input.HSCode,
		CountryOfOrigin: input.CountryOfOrigin,
	}
	if product.VATType == "" {
		product.VATType = domain.VATTypeStandard
	}
	if product.CountryOfOrigin == "" {
		product.CountryOfOrigin = "CN"
	}

	if err := s.repo.InsertProduct(ctx, tx, product); err != nil {
		return nil, fmt.Errorf("CreateProductWithVariants: insert product: %w", err)
	}

	var defaultWarehouseID *uuid.UUID
	if input.TrackInventory {
		selectedWarehouseID, err := s.resolveInitialWarehouseID(ctx, tx, input.TenantID, input.WarehouseID)
		if err != nil {
			if isUndefinedTable(err) {
				s.log.Warn("warehouse tables not present; skipping warehouse stock on create", zap.Error(err))
			} else {
				return nil, fmt.Errorf("CreateProductWithVariants: resolve warehouse: %w", err)
			}
		} else {
			defaultWarehouseID = selectedWarehouseID
		}
	}

	// -------------------------------------------------------------------------
	// 2. Create each variant + seed its inventory row atomically.
	// -------------------------------------------------------------------------
	variants := make([]*domain.Variant, 0, len(input.Variants))

	for _, vi := range input.Variants {
		skuStr, err := generateUniqueJSVariantSKU(ctx, tx)
		if err != nil {
			return nil, fmt.Errorf("CreateProductWithVariants: allocate sku: %w", err)
		}
		variant := &domain.Variant{
			ID:        uuid.New(),
			ProductID: product.ID,
			SKU:       &skuStr,
			Barcode:   vi.Barcode,
			Color:     vi.Color,
			Size:      vi.Size,
			WeightG:   vi.WeightG,
			ImageURL:  vi.ImageURL,
			IsActive:  true,
			MediaURLs: normalizeMediaURLs(vi.MediaURLs),
		}
		costPtr, err := normalizeVariantUnitCost(vi.Cost)
		if err != nil {
			return nil, fmt.Errorf("CreateProductWithVariants: invalid cost for %s: %w", skuStr, err)
		}
		variant.Cost = costPtr

		if err := s.repo.InsertVariant(ctx, tx, variant); err != nil {
			if isUniqueViolation(err) {
				return nil, fmt.Errorf("%w: %s", ErrDuplicateSKU, skuStr)
			}
			return nil, fmt.Errorf("CreateProductWithVariants: insert variant %s: %w", skuStr, err)
		}
		if err := s.repo.ReplaceVariantMedia(ctx, tx, variant.ID, variant.MediaURLs); err != nil {
			return nil, fmt.Errorf("CreateProductWithVariants: set variant media for %s: %w", skuStr, err)
		}

		if err := s.repo.InitInventory(ctx, tx, variant.ID); err != nil {
			return nil, fmt.Errorf("CreateProductWithVariants: init inventory for %s: %w", skuStr, err)
		}

		if vi.Quantity != nil {
			_, err := tx.Exec(ctx, "UPDATE inventory SET quantity_on_hand = $1, updated_at = NOW() AT TIME ZONE 'UTC' WHERE variant_id = $2", *vi.Quantity, variant.ID)
			if err != nil {
				return nil, fmt.Errorf("CreateProductWithVariants: set inventory qty for %s: %w", skuStr, err)
			}
		}
		if input.TrackInventory && defaultWarehouseID != nil {
			qty := 0
			if vi.Quantity != nil {
				qty = *vi.Quantity
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO warehouse_stock
					(id, warehouse_id, variant_id, qty_on_hand, qty_reserved, reorder_point, reorder_qty, created_at, updated_at)
				VALUES
					($1, $2, $3, $4, 0, 0, 0, NOW(), NOW())
				ON CONFLICT (warehouse_id, variant_id)
				DO UPDATE SET qty_on_hand = EXCLUDED.qty_on_hand, updated_at = NOW()`,
				uuid.New(), *defaultWarehouseID, variant.ID, qty,
			)
			if err != nil {
				if isUndefinedTable(err) {
					s.log.Warn("warehouse_stock table missing; skipping warehouse stock write on create", zap.Error(err))
				} else {
					return nil, fmt.Errorf("CreateProductWithVariants: set warehouse stock for %s: %w", skuStr, err)
				}
			}
			_, err = tx.Exec(ctx, `
				UPDATE inventory
				   SET quantity_on_hand = $1, updated_at = NOW()
				 WHERE variant_id = $2
			`, qty, variant.ID)
			if err != nil {
				return nil, fmt.Errorf("CreateProductWithVariants: sync global inventory for %s: %w", skuStr, err)
			}
		}

		var activeChannelID uuid.UUID
		channelErr := tx.QueryRow(ctx, "SELECT id FROM channels WHERE is_active = TRUE ORDER BY created_at ASC LIMIT 1").Scan(&activeChannelID)
		if channelErr == nil {
			if vi.Price != nil && strings.TrimSpace(*vi.Price) != "" {
				p, err := decimal.NewFromString(strings.TrimSpace(*vi.Price))
				if err != nil {
					return nil, fmt.Errorf("CreateProductWithVariants: invalid price for %s: %w", skuStr, err)
				}
				_, err = tx.Exec(ctx, `
					INSERT INTO channel_prices
					    (id, variant_id, channel_id, price, currency, is_active, effective_from, created_at, updated_at)
					VALUES ($1,$2,$3,$4,'AED',TRUE,NOW() AT TIME ZONE 'UTC',NOW() AT TIME ZONE 'UTC',NOW() AT TIME ZONE 'UTC')
					ON CONFLICT (variant_id, channel_id)
					DO UPDATE SET price = EXCLUDED.price, updated_at = NOW() AT TIME ZONE 'UTC'`,
					uuid.New(), variant.ID, activeChannelID, p,
				)
				if err != nil {
					return nil, fmt.Errorf("CreateProductWithVariants: set price for %s: %w", skuStr, err)
				}
			}

			if vi.SalePrice != nil && strings.TrimSpace(*vi.SalePrice) != "" {
				sp, err := decimal.NewFromString(strings.TrimSpace(*vi.SalePrice))
				if err != nil {
					return nil, fmt.Errorf("CreateProductWithVariants: invalid sale price for %s: %w", skuStr, err)
				}
				_, err = tx.Exec(ctx, `
					INSERT INTO price_promotions
					    (id, variant_id, channel_id, promo_price, currency, effective_from, effective_until, is_active, created_at, updated_at)
					VALUES ($1,$2,$3,$4,'AED',NOW() AT TIME ZONE 'UTC',NOW() AT TIME ZONE 'UTC' + INTERVAL '100 years',TRUE,NOW() AT TIME ZONE 'UTC',NOW() AT TIME ZONE 'UTC')`,
					uuid.New(), variant.ID, activeChannelID, sp,
				)
				if err != nil {
					return nil, fmt.Errorf("CreateProductWithVariants: set sale price for %s: %w", skuStr, err)
				}
			}
		}

		variants = append(variants, variant)
	}

	if s.pcs != nil {
		switch {
		case len(input.CategoryIDs) > 0:
			if err := s.pcs.SetProductCategoriesTx(ctx, tx, input.TenantID, product.ID, input.CategoryIDs); err != nil {
				return nil, fmt.Errorf("CreateProductWithVariants: set categories: %w", err)
			}
		case input.CategoryID != nil:
			if err := s.pcs.SetProductCategoryTx(ctx, tx, input.TenantID, product.ID, input.CategoryID); err != nil {
				return nil, fmt.Errorf("CreateProductWithVariants: set category: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("CreateProductWithVariants: commit: %w", err)
	}

	pName := ""
	if product.Name != nil {
		pName = *product.Name
	}
	s.log.Info("product created",
		zap.String("product_id", product.ID.String()),
		zap.String("name", pName),
		zap.Int("variants", len(variants)),
	)

	out := &CreateProductResult{Product: product, Variants: variants}
	if len(input.CategoryIDs) > 0 {
		cid := input.CategoryIDs[0]
		out.CategoryID = &cid
	} else if input.CategoryID != nil {
		cid := *input.CategoryID
		out.CategoryID = &cid
	}
	return out, nil
}

// GetProduct returns a product with all its variants.
func (s *ProductService) GetProduct(ctx context.Context, id uuid.UUID) (*CreateProductResult, error) {
	product, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProductNotFound
		}
		return nil, err
	}

	variants, err := s.repo.ListVariantsByProduct(ctx, id)
	if err != nil {
		return nil, err
	}

	out := &CreateProductResult{Product: product, Variants: variants}
	if s.pcm != nil {
		ids, err := s.pcm.ListCollectionIDsForProduct(ctx, id)
		switch {
		case err != nil:
			s.log.Warn("ListCollectionIDsForProduct failed; returning empty collection_ids",
				zap.String("product_id", id.String()), zap.Error(err))
			out.CollectionIDs = []uuid.UUID{}
		default:
			// Always a non-nil slice so JSON is [] (not omitted) — stable admin reload/sync.
			out.CollectionIDs = append([]uuid.UUID{}, ids...)
		}
	}
	if s.pcs != nil {
		cid, err := s.pcs.FirstCategoryIDForProduct(ctx, id)
		switch {
		case err != nil:
			s.log.Warn("FirstCategoryIDForProduct failed; omitting category_id",
				zap.String("product_id", id.String()), zap.Error(err))
		case cid != nil:
			out.CategoryID = cid
		}
	}
	return out, nil
}

// SetPrice sets or replaces the selling price for a variant on a specific channel.
// If a price already exists for that (variant, channel) pair it is overwritten.
func (s *ProductService) SetPrice(ctx context.Context, input SetPriceInput) (*domain.ChannelPrice, error) {
	if input.Price.IsNegative() {
		return nil, fmt.Errorf("price cannot be negative")
	}
	if input.Currency == "" {
		input.Currency = "AED"
	}

	cp := &domain.ChannelPrice{
		ID:             uuid.New(),
		VariantID:      input.VariantID,
		ChannelID:      input.ChannelID,
		Price:          input.Price,
		Currency:       input.Currency,
		IsActive:       true,
		EffectiveUntil: input.EffectiveUntil,
	}

	if err := s.repo.UpsertChannelPrice(ctx, cp); err != nil {
		return nil, fmt.Errorf("SetPrice: %w", err)
	}

	s.log.Info("channel price set",
		zap.String("variant_id", cp.VariantID.String()),
		zap.String("channel_id", cp.ChannelID.String()),
		zap.String("price", cp.Price.String()),
	)

	return cp, nil
}

// GetPrice fetches the current active price for a variant + channel.
func (s *ProductService) GetPrice(
	ctx context.Context,
	variantID, channelID uuid.UUID,
) (*domain.ChannelPrice, error) {
	cp, err := s.repo.GetChannelPrice(ctx, variantID, channelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no active price found for variant %s on channel %s", variantID, channelID)
		}
		return nil, err
	}
	return cp, nil
}

func (s *ProductService) ListProducts(ctx context.Context, filters domain.ProductListFilters) (*domain.PageResponse[domain.ProductListItem], error) {
	listRepo, ok := s.repo.(interface {
		ListProducts(ctx context.Context, filters domain.ProductListFilters) ([]domain.ProductListItem, int, error)
	})
	if !ok {
		return nil, fmt.Errorf("ListProducts: repository does not support list queries")
	}

	filters.Page = normalizePage(filters.Page)
	filters.PageSize = normalizePageSize(filters.PageSize)

	items, total, err := listRepo.ListProducts(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("ListProducts: %w", err)
	}

	if filters.PublicCatalog {
		for i := range items {
			items[i].Cost = nil
		}
	}

	return &domain.PageResponse[domain.ProductListItem]{
		Items: items,
		Total: total,
	}, nil
}

func (s *ProductService) UpsertDefaultVariantForProduct(ctx context.Context, productID uuid.UUID, input UpsertVariantInput) (*domain.Variant, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("UpsertDefaultVariantForProduct begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("UpsertDefaultVariantForProduct rollback", zap.Error(rbErr))
		}
	}()

	variants, err := s.repo.ListVariantsByProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("UpsertDefaultVariantForProduct list variants: %w", err)
	}

	if len(variants) == 0 {
		skuStr, err := generateUniqueJSVariantSKU(ctx, tx)
		if err != nil {
			return nil, fmt.Errorf("UpsertDefaultVariantForProduct allocate sku: %w", err)
		}
		v := &domain.Variant{
			ID:        uuid.New(),
			ProductID: productID,
			SKU:       &skuStr,
			Color:     input.Color,
			Size:      input.Size,
			ImageURL:  input.ImageURL,
			IsActive:  true,
			MediaURLs: normalizeMediaURLs(input.MediaURLs),
		}
		costPtr, err := normalizeVariantUnitCost(input.Cost)
		if err != nil {
			return nil, fmt.Errorf("UpsertDefaultVariantForProduct invalid cost: %w", err)
		}
		v.Cost = costPtr
		if err := s.repo.InsertVariant(ctx, tx, v); err != nil {
			return nil, fmt.Errorf("UpsertDefaultVariantForProduct insert: %w", err)
		}
		if err := s.repo.ReplaceVariantMedia(ctx, tx, v.ID, v.MediaURLs); err != nil {
			return nil, fmt.Errorf("UpsertDefaultVariantForProduct set media: %w", err)
		}
		if err := s.repo.InitInventory(ctx, tx, v.ID); err != nil {
			return nil, fmt.Errorf("UpsertDefaultVariantForProduct init inventory: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("UpsertDefaultVariantForProduct commit: %w", err)
		}
		return v, nil
	}

	v := variants[0]
	if s := strings.ToUpper(strings.TrimSpace(input.SKU)); s != "" {
		v.SKU = &s
	}
	if input.Color != nil {
		v.Color = input.Color
	}
	if input.Size != nil {
		v.Size = input.Size
	}
	if input.ImageURL != nil {
		v.ImageURL = input.ImageURL
	}
	costPtr, err := normalizeVariantUnitCost(input.Cost)
	if err != nil {
		return nil, fmt.Errorf("UpsertDefaultVariantForProduct invalid cost: %w", err)
	}
	v.Cost = costPtr
	v.MediaURLs = normalizeMediaURLs(input.MediaURLs)

	if err := s.repo.UpdateVariant(ctx, tx, v); err != nil {
		return nil, fmt.Errorf("UpsertDefaultVariantForProduct update: %w", err)
	}
	if err := s.repo.ReplaceVariantMedia(ctx, tx, v.ID, v.MediaURLs); err != nil {
		return nil, fmt.Errorf("UpsertDefaultVariantForProduct update media: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("UpsertDefaultVariantForProduct commit: %w", err)
	}
	return v, nil
}

func (s *ProductService) CreateVariantForProduct(ctx context.Context, productID uuid.UUID, input UpsertVariantInput) (*domain.Variant, error) {
	if _, err := s.repo.GetProductByID(ctx, productID); err != nil {
		return nil, fmt.Errorf("CreateVariantForProduct get product: %w", err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("CreateVariantForProduct begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("CreateVariantForProduct rollback", zap.Error(rbErr))
		}
	}()

	skuStr, err := generateUniqueJSVariantSKU(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("CreateVariantForProduct allocate sku: %w", err)
	}

	v := &domain.Variant{
		ID:        uuid.New(),
		ProductID: productID,
		SKU:       &skuStr,
		Color:     input.Color,
		Size:      input.Size,
		ImageURL:  input.ImageURL,
		IsActive:  true,
		MediaURLs: normalizeMediaURLs(input.MediaURLs),
	}
	costPtr, err := normalizeVariantUnitCost(input.Cost)
	if err != nil {
		return nil, fmt.Errorf("CreateVariantForProduct invalid cost: %w", err)
	}
	v.Cost = costPtr
	if err := s.repo.InsertVariant(ctx, tx, v); err != nil {
		return nil, fmt.Errorf("CreateVariantForProduct insert: %w", err)
	}
	if err := s.repo.ReplaceVariantMedia(ctx, tx, v.ID, v.MediaURLs); err != nil {
		return nil, fmt.Errorf("CreateVariantForProduct set media: %w", err)
	}
	if err := s.repo.InitInventory(ctx, tx, v.ID); err != nil {
		return nil, fmt.Errorf("CreateVariantForProduct init inventory: %w", err)
	}

	if input.Quantity != nil {
		_, err := tx.Exec(ctx, "UPDATE inventory SET quantity_on_hand = $1, updated_at = NOW() AT TIME ZONE 'UTC' WHERE variant_id = $2", *input.Quantity, v.ID)
		if err != nil {
			return nil, fmt.Errorf("CreateVariantForProduct inventory: %w", err)
		}
	}
	if input.Price != nil {
		priceRaw := strings.TrimSpace(*input.Price)
		if priceRaw != "" {
			p, err := decimal.NewFromString(priceRaw)
			if err != nil {
				return nil, fmt.Errorf("CreateVariantForProduct invalid price: %w", err)
			}
			var channelID uuid.UUID
			if err := tx.QueryRow(ctx, "SELECT id FROM channels WHERE is_active = TRUE ORDER BY created_at ASC LIMIT 1").Scan(&channelID); err == nil {
				if _, err := tx.Exec(ctx, `
					INSERT INTO channel_prices
					    (id, variant_id, channel_id, price, currency, is_active, effective_from, created_at, updated_at)
					VALUES ($1,$2,$3,$4,'AED',TRUE,NOW() AT TIME ZONE 'UTC',NOW() AT TIME ZONE 'UTC',NOW() AT TIME ZONE 'UTC')
					ON CONFLICT (variant_id, channel_id)
					DO UPDATE SET price = EXCLUDED.price, updated_at = NOW() AT TIME ZONE 'UTC'`,
					uuid.New(), v.ID, channelID, p,
				); err != nil {
					return nil, fmt.Errorf("CreateVariantForProduct insert channel price: %w", err)
				}
			}
		}
	}
	if input.SalePrice != nil {
		saleRaw := strings.TrimSpace(*input.SalePrice)
		if saleRaw != "" {
			sp, err := decimal.NewFromString(saleRaw)
			if err != nil {
				return nil, fmt.Errorf("CreateVariantForProduct invalid sale price: %w", err)
			}
			var channelID uuid.UUID
			if err := tx.QueryRow(ctx, "SELECT id FROM channels WHERE is_active = TRUE ORDER BY created_at ASC LIMIT 1").Scan(&channelID); err == nil {
				if _, err := tx.Exec(ctx, `
					INSERT INTO price_promotions
					    (id, variant_id, channel_id, promo_price, currency, effective_from, effective_until, is_active, created_at, updated_at)
					VALUES ($1,$2,$3,$4,'AED',NOW() AT TIME ZONE 'UTC',NOW() AT TIME ZONE 'UTC' + INTERVAL '100 years',TRUE,NOW() AT TIME ZONE 'UTC',NOW() AT TIME ZONE 'UTC')`,
					uuid.New(), v.ID, channelID, sp,
				); err != nil {
					return nil, fmt.Errorf("CreateVariantForProduct insert sale price: %w", err)
				}
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("CreateVariantForProduct commit: %w", err)
	}
	return v, nil
}

func (s *ProductService) UpdateVariant(ctx context.Context, variantID uuid.UUID, input UpsertVariantInput) error {
	if strings.TrimSpace(input.SKU) == "" {
		return fmt.Errorf("SKU is required")
	}
	existing, err := s.repo.GetVariantByID(ctx, variantID)
	if err != nil {
		return fmt.Errorf("UpdateVariant get variant: %w", err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("UpdateVariant begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("UpdateVariant rollback", zap.Error(rbErr))
		}
	}()

	sku := strings.ToUpper(strings.TrimSpace(input.SKU))
	existing.SKU = &sku
	// Partial PATCH: only overwrite fields present in the JSON body. Omitting image_url /
	// media_urls must not clear them (e.g. admin list price-only updates).
	if input.Color != nil {
		existing.Color = input.Color
	}
	if input.Size != nil {
		existing.Size = input.Size
	}
	if input.ImageURL != nil {
		existing.ImageURL = input.ImageURL
	}
	if input.Cost != nil {
		costPtr, err := normalizeVariantUnitCost(input.Cost)
		if err != nil {
			return fmt.Errorf("UpdateVariant cost: %w", err)
		}
		existing.Cost = costPtr
	}
	if input.MediaURLs != nil {
		existing.MediaURLs = normalizeMediaURLs(input.MediaURLs)
	}
	if err := s.repo.UpdateVariant(ctx, tx, existing); err != nil {
		return fmt.Errorf("UpdateVariant persist: %w", err)
	}
	if input.MediaURLs != nil {
		if err := s.repo.ReplaceVariantMedia(ctx, tx, existing.ID, existing.MediaURLs); err != nil {
			return fmt.Errorf("UpdateVariant media: %w", err)
		}
	}

	// -------------------------------------------------------------
	// 2. Cascade direct cross-domain overwrites (Inventory, Prices)
	// -------------------------------------------------------------

	if input.Quantity != nil {
		_, err := tx.Exec(ctx, "UPDATE inventory SET quantity_on_hand = $1, updated_at = NOW() AT TIME ZONE 'UTC' WHERE variant_id = $2", *input.Quantity, existing.ID)
		if err != nil {
			return fmt.Errorf("UpdateVariant inventory: %w", err)
		}
		var warehouseID uuid.UUID
		err = tx.QueryRow(ctx, `
			SELECT warehouse_id
			  FROM warehouse_stock
			 WHERE variant_id = $1
			 ORDER BY created_at ASC
			 LIMIT 1
		`, existing.ID).Scan(&warehouseID)
		if isUndefinedTable(err) {
			s.log.Warn("warehouse tables not present; skipping warehouse stock sync on variant update", zap.Error(err))
			goto skipWarehouseSync
		}
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
				SELECT id
				  FROM warehouses
				 ORDER BY priority ASC, created_at ASC
				 LIMIT 1
			`).Scan(&warehouseID)
		}
		if err != nil {
			return fmt.Errorf("UpdateVariant resolve default warehouse: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO warehouse_stock
				(id, warehouse_id, variant_id, qty_on_hand, qty_reserved, reorder_point, reorder_qty, created_at, updated_at)
			VALUES
				($1, $2, $3, $4, 0, 0, 0, NOW(), NOW())
			ON CONFLICT (warehouse_id, variant_id)
			DO UPDATE SET qty_on_hand = EXCLUDED.qty_on_hand, updated_at = NOW()
		`, uuid.New(), warehouseID, existing.ID, *input.Quantity)
		if err != nil {
			if isUndefinedTable(err) {
				s.log.Warn("warehouse_stock table missing; skipping warehouse stock sync on variant update", zap.Error(err))
				goto skipWarehouseSync
			}
			return fmt.Errorf("UpdateVariant sync warehouse stock: %w", err)
		}
	skipWarehouseSync:
	}

	if input.Price != nil {
		priceRaw := strings.TrimSpace(*input.Price)
		if priceRaw != "" {
			p, err := decimal.NewFromString(priceRaw)
			if err != nil {
				return fmt.Errorf("UpdateVariant invalid price: %w", err)
			}
			tag, err := tx.Exec(ctx, "UPDATE channel_prices SET price = $1, updated_at = NOW() AT TIME ZONE 'UTC' WHERE variant_id = $2", p, existing.ID)
			if err != nil {
				return fmt.Errorf("UpdateVariant channel price: %w", err)
			}
			if tag.RowsAffected() == 0 {
				var channelID uuid.UUID
				if err := tx.QueryRow(ctx, "SELECT id FROM channels WHERE is_active = TRUE ORDER BY created_at ASC LIMIT 1").Scan(&channelID); err != nil {
					return fmt.Errorf("UpdateVariant select active channel: %w", err)
				}
				_, err := tx.Exec(ctx, `
					INSERT INTO channel_prices
					    (id, variant_id, channel_id, price, currency, is_active, effective_from, created_at, updated_at)
					VALUES ($1,$2,$3,$4,'AED',TRUE,NOW() AT TIME ZONE 'UTC',NOW() AT TIME ZONE 'UTC',NOW() AT TIME ZONE 'UTC')
					ON CONFLICT (variant_id, channel_id)
					DO UPDATE SET price = EXCLUDED.price, updated_at = NOW() AT TIME ZONE 'UTC'`,
					uuid.New(), existing.ID, channelID, p,
				)
				if err != nil {
					return fmt.Errorf("UpdateVariant insert channel price: %w", err)
				}
			}
		}
	}

	if input.SalePrice != nil {
		saleRaw := strings.TrimSpace(*input.SalePrice)
		if saleRaw == "" {
			_, err := tx.Exec(ctx, "UPDATE price_promotions SET is_active = FALSE, updated_at = NOW() AT TIME ZONE 'UTC' WHERE variant_id = $1 AND is_active = TRUE", existing.ID)
			if err != nil {
				return fmt.Errorf("UpdateVariant clear sale price: %w", err)
			}
		} else {
			sp, err := decimal.NewFromString(saleRaw)
			if err != nil {
				return fmt.Errorf("UpdateVariant invalid sale price: %w", err)
			}
			var channelID uuid.UUID
			if err := tx.QueryRow(ctx, "SELECT id FROM channels WHERE is_active = TRUE ORDER BY created_at ASC LIMIT 1").Scan(&channelID); err != nil {
				return fmt.Errorf("UpdateVariant select active channel for sale: %w", err)
			}
			_, err = tx.Exec(ctx, "UPDATE price_promotions SET is_active = FALSE, updated_at = NOW() AT TIME ZONE 'UTC' WHERE variant_id = $1 AND channel_id = $2 AND is_active = TRUE", existing.ID, channelID)
			if err != nil {
				return fmt.Errorf("UpdateVariant deactivate existing sale: %w", err)
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO price_promotions
				    (id, variant_id, channel_id, promo_price, currency, effective_from, effective_until, is_active, created_at, updated_at)
				VALUES ($1,$2,$3,$4,'AED',NOW() AT TIME ZONE 'UTC',NOW() AT TIME ZONE 'UTC' + INTERVAL '100 years',TRUE,NOW() AT TIME ZONE 'UTC',NOW() AT TIME ZONE 'UTC')`,
				uuid.New(), existing.ID, channelID, sp,
			)
			if err != nil {
				return fmt.Errorf("UpdateVariant insert sale price: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("UpdateVariant commit: %w", err)
	}
	return nil
}

func (s *ProductService) DeleteVariant(ctx context.Context, variantID uuid.UUID) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("DeleteVariant begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("DeleteVariant rollback", zap.Error(rbErr))
		}
	}()
	if err := s.repo.DeleteVariant(ctx, tx, variantID); err != nil {
		return fmt.Errorf("DeleteVariant persist: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("DeleteVariant commit: %w", err)
	}
	return nil
}

func (s *ProductService) DeleteProduct(ctx context.Context, productID uuid.UUID) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("DeleteProduct begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("DeleteProduct rollback", zap.Error(rbErr))
		}
	}()

	updates := map[string]any{
		"status": domain.StatusArchived,
	}
	if err := s.repo.PatchProduct(ctx, tx, productID, updates); err != nil {
		return fmt.Errorf("DeleteProduct patch: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("DeleteProduct commit: %w", err)
	}
	return nil
}

func (s *ProductService) DuplicateProduct(ctx context.Context, productID uuid.UUID) (*domain.Product, error) {
	src, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("DuplicateProduct source: %w", err)
	}
	srcVariants, err := s.repo.ListVariantsByProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("DuplicateProduct source variants: %w", err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("DuplicateProduct begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			s.log.Error("DuplicateProduct rollback", zap.Error(rbErr))
		}
	}()

	dupName := "Untitled-draft"
	if src.Name != nil && strings.TrimSpace(*src.Name) != "" {
		dupName = strings.TrimSpace(*src.Name) + "-draft"
	}
	dupSlug, err := s.repo.ResolveUniqueProductSlug(ctx, tx, slugify(dupName), nil)
	if err != nil {
		return nil, fmt.Errorf("DuplicateProduct resolve slug: %w", err)
	}
	dup := &domain.Product{
		ID:              uuid.New(),
		Name:            &dupName,
		Slug:            &dupSlug,
		NameAR:          src.NameAR,
		Description:     src.Description,
		Brand:           src.Brand,
		Category:        src.Category,
		SubCategory:     src.SubCategory,
		Status:          domain.StatusDraft,
		VATType:         src.VATType,
		HSCode:          src.HSCode,
		CountryOfOrigin: src.CountryOfOrigin,
	}
	if err := s.repo.InsertProduct(ctx, tx, dup); err != nil {
		return nil, fmt.Errorf("DuplicateProduct insert product: %w", err)
	}

	for _, v := range srcVariants {
		newSku, err := generateUniqueJSVariantSKU(ctx, tx)
		if err != nil {
			return nil, fmt.Errorf("DuplicateProduct sku: %w", err)
		}

		nv := &domain.Variant{
			ID:        uuid.New(),
			ProductID: dup.ID,
			SKU:       &newSku,
			Barcode:   nil,
			Color:     v.Color,
			Size:      v.Size,
			WeightG:   v.WeightG,
			ImageURL:  v.ImageURL,
			IsActive:  true,
		}
		if v.Cost != nil {
			c := *v.Cost
			nv.Cost = &c
		}
		if err := s.repo.InsertVariant(ctx, tx, nv); err != nil {
			return nil, fmt.Errorf("DuplicateProduct insert variant: %w", err)
		}
		mediaURLs := normalizeMediaURLs(v.MediaURLs)
		if err := s.repo.ReplaceVariantMedia(ctx, tx, nv.ID, mediaURLs); err != nil {
			return nil, fmt.Errorf("DuplicateProduct variant media: %w", err)
		}
		if err := s.repo.InitInventory(ctx, tx, nv.ID); err != nil {
			return nil, fmt.Errorf("DuplicateProduct init inventory: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("DuplicateProduct commit: %w", err)
	}
	return dup, nil
}

// =============================================================================
// Helpers
// =============================================================================

func randomJSVariant8Digit() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("JS-%08d", (time.Now().UnixNano()%90000000)+10000000)
	}
	n := binary.BigEndian.Uint32(b[:])
	n = n%90000000 + 10000000
	return fmt.Sprintf("JS-%08d", n)
}

func generateUniqueJSVariantSKU(ctx context.Context, tx pgx.Tx) (string, error) {
	for range 80 {
		sku := randomJSVariant8Digit()
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM variants WHERE sku = $1)`, sku).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return sku, nil
		}
	}
	return "", fmt.Errorf("could not allocate unique JS variant SKU")
}

func normalizeVariantUnitCost(cost *string) (*string, error) {
	if cost == nil {
		return nil, nil
	}
	raw := strings.TrimSpace(*cost)
	if raw == "" {
		return nil, nil
	}
	if _, err := decimal.NewFromString(raw); err != nil {
		return nil, fmt.Errorf("invalid cost: %w", err)
	}
	return &raw, nil
}

func validateCreateProductInput(input CreateProductInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("product name is required")
	}
	if len(input.Variants) == 0 {
		return fmt.Errorf("at least one variant is required")
	}
	return nil
}

func (s *ProductService) resolveInitialWarehouseID(
	ctx context.Context,
	tx pgx.Tx,
	tenantID uuid.UUID,
	preferred *uuid.UUID,
) (*uuid.UUID, error) {
	if preferred != nil && *preferred != uuid.Nil {
		return preferred, nil
	}
	var warehouseID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT id
		  FROM warehouses
		 WHERE tenant_id = $1
		 ORDER BY priority ASC, created_at ASC
		 LIMIT 1
	`, tenantID).Scan(&warehouseID)
	if isUndefinedTable(err) {
		return nil, err
	}
	if err == nil {
		return &warehouseID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO warehouses
			(id, tenant_id, name, type, address, city, country, is_active, priority, created_at, updated_at)
		VALUES
			($1, $2, 'Default Warehouse', 'warehouse', 'N/A', 'Dubai', 'AE', TRUE, 1, NOW(), NOW())
		RETURNING id
	`, uuid.New(), tenantID).Scan(&warehouseID)
	if err != nil {
		return nil, err
	}
	return &warehouseID, nil
}

func isUndefinedTable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42P01"
}

// isUniqueViolation detects PostgreSQL unique constraint error code 23505.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
}

var nonSlugChars = regexp.MustCompile(`[^a-z0-9\s-]`)
var multiWhitespace = regexp.MustCompile(`[\s_-]+`)

func slugify(s string) string {
	out := strings.ToLower(strings.TrimSpace(s))
	out = nonSlugChars.ReplaceAllString(out, "")
	out = multiWhitespace.ReplaceAllString(out, "-")
	out = strings.Trim(out, "-")
	if out == "" {
		return "product"
	}
	return out
}

func normalizeMediaURLs(urls []string) []string {
	if len(urls) == 0 {
		return nil
	}
	out := make([]string, 0, len(urls))
	seen := make(map[string]struct{}, len(urls))
	for _, raw := range urls {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
