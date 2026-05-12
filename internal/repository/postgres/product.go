package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// ProductRepository handles products, variants, channel prices, and inventory seeding.
type ProductRepository struct {
	pool *pgxpool.Pool
}

// =============================================================================
// Products
// =============================================================================

// InsertProduct inserts a new product row inside the given transaction.
func (r *ProductRepository) InsertProduct(ctx context.Context, tx pgx.Tx, p *domain.Product) error {
	const q = `
		INSERT INTO products
		    (id, name, slug, name_ar, description, brand, category, sub_category,
		     status, vat_type, hs_code, country_of_origin, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,
		        NOW() AT TIME ZONE 'UTC', NOW() AT TIME ZONE 'UTC')`

	_, err := tx.Exec(ctx, q,
		p.ID, p.Name, p.Slug, p.NameAR, p.Description, p.Brand,
		p.Category, p.SubCategory, p.Status, p.VATType,
		p.HSCode, p.CountryOfOrigin,
	)
	if err != nil {
		return fmt.Errorf("InsertProduct(%s): %w", p.ID, err)
	}
	return nil
}

// InsertDraftProduct creates a new draft product with an empty name.
func (r *ProductRepository) InsertDraftProduct(ctx context.Context, tx pgx.Tx) (uuid.UUID, error) {
	id := uuid.New()
	const q = `
		INSERT INTO products (id, status, created_at, updated_at)
		VALUES ($1, 'draft', NOW() AT TIME ZONE 'UTC', NOW() AT TIME ZONE 'UTC')`
	_, err := tx.Exec(ctx, q, id)
	return id, err
}

// PatchProduct applies partial updates to a product.
func (r *ProductRepository) PatchProduct(ctx context.Context, tx pgx.Tx, id uuid.UUID, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	setClauses := []string{}
	args := []any{}
	argID := 1

	for col, val := range updates {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, argID))
		args = append(args, val)
		argID++
	}

	setClauses = append(setClauses, "updated_at = NOW() AT TIME ZONE 'UTC'")
	args = append(args, id)
	q := fmt.Sprintf("UPDATE products SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argID)

	_, err := tx.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("PatchProduct(%s): %w", id, err)
	}
	return nil
}

// GetProductByID fetches a product by primary key.
func (r *ProductRepository) GetProductByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	const q = `
		SELECT id, name, slug, name_ar, description, brand, category, sub_category,
		       status, vat_type, hs_code, country_of_origin, created_at, updated_at
		  FROM products
		 WHERE id = $1`

	p := &domain.Product{}
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&p.ID, &p.Name, &p.Slug, &p.NameAR, &p.Description, &p.Brand,
		&p.Category, &p.SubCategory, &p.Status, &p.VATType,
		&p.HSCode, &p.CountryOfOrigin, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetProductByID(%s): %w", id, err)
	}
	return p, nil
}

func (r *ProductRepository) ResolveUniqueProductSlug(ctx context.Context, tx pgx.Tx, base string, excludeID *uuid.UUID) (string, error) {
	slug := strings.TrimSpace(strings.ToLower(base))
	if slug == "" {
		slug = "product"
	}

	const q = `
		SELECT EXISTS (
			SELECT 1
			FROM products
			WHERE slug = $1
			  AND ($2::uuid IS NULL OR id <> $2::uuid)
		)`

	candidate := slug
	suffix := 1
	for {
		var exists bool
		if err := tx.QueryRow(ctx, q, candidate, excludeID).Scan(&exists); err != nil {
			return "", fmt.Errorf("ResolveUniqueProductSlug(%s): %w", candidate, err)
		}
		if !exists {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", slug, suffix)
		suffix++
	}
}

// =============================================================================
// Variants
// =============================================================================

// InsertVariant inserts a variant inside the given transaction.
func (r *ProductRepository) InsertVariant(ctx context.Context, tx pgx.Tx, v *domain.Variant) error {
	const q = `
		INSERT INTO variants
		    (id, product_id, sku, barcode, color, size, weight_g,
		     image_url, unit_cost, is_active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
		        NOW() AT TIME ZONE 'UTC', NOW() AT TIME ZONE 'UTC')`

	_, err := tx.Exec(ctx, q,
		v.ID, v.ProductID, v.SKU, v.Barcode, v.Color,
		v.Size, v.WeightG, v.ImageURL, v.Cost, v.IsActive,
	)
	if err != nil {
		return fmt.Errorf("InsertVariant(%s, sku=%v): %w", v.ID, v.SKU, err)
	}
	return nil
}

// GetVariantBySKU looks up a variant by its SKU code, joining in the product.
func (r *ProductRepository) GetVariantBySKU(ctx context.Context, sku string) (*domain.Variant, error) {
	const q = `
		SELECT v.id, v.product_id, v.sku, v.barcode, v.color, v.size,
		       v.weight_g, v.image_url, v.unit_cost::text, v.is_active, v.created_at, v.updated_at
		  FROM variants v
		 WHERE v.sku = $1`

	v := &domain.Variant{}
	var unitCost sql.NullString
	err := r.pool.QueryRow(ctx, q, sku).Scan(
		&v.ID, &v.ProductID, &v.SKU, &v.Barcode, &v.Color, &v.Size,
		&v.WeightG, &v.ImageURL, &unitCost, &v.IsActive, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetVariantBySKU(%s): %w", sku, err)
	}
	if unitCost.Valid && strings.TrimSpace(unitCost.String) != "" {
		s := strings.TrimSpace(unitCost.String)
		v.Cost = &s
	}

	// Load media
	mediaQ := `SELECT url FROM variant_media WHERE variant_id = $1 ORDER BY sort_order, created_at`
	mRows, err := r.pool.Query(ctx, mediaQ, v.ID)
	if err == nil {
		defer mRows.Close()
		for mRows.Next() {
			var url string
			if err := mRows.Scan(&url); err == nil {
				v.MediaURLs = append(v.MediaURLs, url)
			}
		}
	}

	return v, nil
}

// GetVariantByID looks up a variant by primary key.
func (r *ProductRepository) GetVariantByID(ctx context.Context, id uuid.UUID) (*domain.Variant, error) {
	const q = `
		SELECT id, product_id, sku, barcode, color, size,
		       weight_g, image_url, unit_cost::text, is_active, created_at, updated_at
		  FROM variants
		 WHERE id = $1`

	v := &domain.Variant{}
	var unitCost sql.NullString
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&v.ID, &v.ProductID, &v.SKU, &v.Barcode, &v.Color, &v.Size,
		&v.WeightG, &v.ImageURL, &unitCost, &v.IsActive, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetVariantByID(%s): %w", id, err)
	}
	if unitCost.Valid && strings.TrimSpace(unitCost.String) != "" {
		s := strings.TrimSpace(unitCost.String)
		v.Cost = &s
	}

	// Load media
	mediaQ := `SELECT url FROM variant_media WHERE variant_id = $1 ORDER BY sort_order, created_at`
	mRows, err := r.pool.Query(ctx, mediaQ, v.ID)
	if err == nil {
		defer mRows.Close()
		for mRows.Next() {
			var url string
			if err := mRows.Scan(&url); err == nil {
				v.MediaURLs = append(v.MediaURLs, url)
			}
		}
	}

	return v, nil
}

// ListVariantsByProduct returns all variants for a given product.
func (r *ProductRepository) ListVariantsByProduct(ctx context.Context, productID uuid.UUID) ([]*domain.Variant, error) {
	const q = `
		SELECT id, product_id, sku, barcode, color, size,
		       weight_g, image_url,
		       variants.unit_cost::text AS unit_cost,
		       (
		           SELECT cp.price::text
		           FROM channel_prices cp
		           JOIN channels c ON c.id = cp.channel_id AND c.is_active = TRUE
		           WHERE cp.variant_id = variants.id
		             AND cp.is_active = TRUE
		           ORDER BY cp.effective_from DESC
		           LIMIT 1
		       ) AS price,
		       (
		           SELECT pp.promo_price::text
		           FROM price_promotions pp
		           WHERE pp.variant_id = variants.id
		             AND pp.is_active = TRUE
		             AND pp.effective_from <= NOW() AT TIME ZONE 'UTC'
		             AND pp.effective_until > NOW() AT TIME ZONE 'UTC'
		           ORDER BY pp.effective_from DESC
		           LIMIT 1
		       ) AS sale_price,
		       COALESCE((
		           SELECT i.quantity_on_hand
		           FROM inventory i
		           WHERE i.variant_id = variants.id
		           LIMIT 1
		       ), 0) AS quantity,
		       is_active, created_at, updated_at
		  FROM variants
		 WHERE product_id = $1
		   AND is_active = TRUE
		 ORDER BY size, color`

	rows, err := r.pool.Query(ctx, q, productID)
	if err != nil {
		return nil, fmt.Errorf("ListVariantsByProduct(%s): %w", productID, err)
	}
	defer rows.Close()

	var variants []*domain.Variant
	variantIndexByID := make(map[uuid.UUID]*domain.Variant)
	for rows.Next() {
		v := &domain.Variant{}
		var price sql.NullString
		var salePrice sql.NullString
		var unitCost sql.NullString
		var qty int
		if err := rows.Scan(
			&v.ID, &v.ProductID, &v.SKU, &v.Barcode, &v.Color, &v.Size,
			&v.WeightG, &v.ImageURL, &unitCost, &price, &salePrice, &qty, &v.IsActive, &v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if unitCost.Valid && strings.TrimSpace(unitCost.String) != "" {
			s := strings.TrimSpace(unitCost.String)
			v.Cost = &s
		}
		if price.Valid {
			val := price.String
			v.Price = &val
		}
		if salePrice.Valid {
			val := salePrice.String
			v.SalePrice = &val
		}
		v.Quantity = &qty
		variants = append(variants, v)
		variantIndexByID[v.ID] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// -------------------------------------------------------------------------
	// 3. Eagerly load MediaURLs for each variant in bulk.
	// -------------------------------------------------------------------------
	if len(variants) > 0 {
		variantIDs := make([]uuid.UUID, len(variants))
		for i, v := range variants {
			variantIDs[i] = v.ID
		}

		mediaQ := `
			SELECT variant_id, url
			FROM variant_media
			WHERE variant_id = ANY($1)
			ORDER BY variant_id, sort_order, created_at`
		mRows, err := r.pool.Query(ctx, mediaQ, variantIDs)
		if err != nil {
			return nil, fmt.Errorf("ListVariantsByProduct media: %w", err)
		}
		defer mRows.Close()

		mediaMap := make(map[uuid.UUID][]string)
		for mRows.Next() {
			var vid uuid.UUID
			var url string
			if err := mRows.Scan(&vid, &url); err != nil {
				return nil, err
			}
			mediaMap[vid] = append(mediaMap[vid], url)
		}
		for _, v := range variants {
			v.MediaURLs = mediaMap[v.ID]
			if v.ImageURL == nil && len(v.MediaURLs) > 0 {
				first := v.MediaURLs[0]
				v.ImageURL = &first
			}
		}
	}

	return variants, nil
}

func (r *ProductRepository) ReplaceVariantMedia(ctx context.Context, tx pgx.Tx, variantID uuid.UUID, urls []string) error {
	// 1. Delete existing
	if _, err := tx.Exec(ctx, "DELETE FROM variant_media WHERE variant_id = $1", variantID); err != nil {
		return fmt.Errorf("ReplaceVariantMedia delete: %w", err)
	}
	if len(urls) == 0 {
		return nil
	}

	// 2. Insert new
	// We use a manual loop or a batch insert. Given it's usually < 10 images, loop is fine.
	for i, url := range urls {
		const q = `
			INSERT INTO variant_media (variant_id, url, sort_order)
			VALUES ($1, $2, $3)
			ON CONFLICT (variant_id, url) DO NOTHING`
		if _, err := tx.Exec(ctx, q, variantID, url, i); err != nil {
			return fmt.Errorf("ReplaceVariantMedia insert %s: %w", url, err)
		}
	}

	return nil
}

func (r *ProductRepository) UpdateVariant(ctx context.Context, tx pgx.Tx, v *domain.Variant) error {
	const q = `
		UPDATE variants
		   SET sku = $2,
		       color = $3,
		       size = $4,
		       image_url = $5,
		       unit_cost = $6::numeric,
		       updated_at = NOW() AT TIME ZONE 'UTC'
		 WHERE id = $1`
	_, err := tx.Exec(ctx, q, v.ID, v.SKU, v.Color, v.Size, v.ImageURL, v.Cost)
	if err != nil {
		return fmt.Errorf("UpdateVariant(%s): %w", v.ID, err)
	}
	return nil
}

func (r *ProductRepository) DeleteVariant(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	// Remove dependent mutable rows first, then delete the variant.
	// Historical rows (orders/movements/etc.) are intentionally not removed;
	// if they exist, the final DELETE will fail and the caller can surface that.
	if _, err := tx.Exec(ctx, `DELETE FROM price_promotions WHERE variant_id = $1`, id); err != nil {
		return fmt.Errorf("DeleteVariant(%s) promotions: %w", id, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM channel_prices WHERE variant_id = $1`, id); err != nil {
		return fmt.Errorf("DeleteVariant(%s) channel_prices: %w", id, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM stock_reservations WHERE variant_id = $1`, id); err != nil {
		return fmt.Errorf("DeleteVariant(%s) stock_reservations: %w", id, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM platform_products WHERE variant_id = $1`, id); err != nil {
		return fmt.Errorf("DeleteVariant(%s) platform_products: %w", id, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM batch_items WHERE variant_id = $1`, id); err != nil {
		return fmt.Errorf("DeleteVariant(%s) batch_items: %w", id, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM variant_media WHERE variant_id = $1`, id); err != nil {
		return fmt.Errorf("DeleteVariant(%s) variant_media: %w", id, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM inventory WHERE variant_id = $1`, id); err != nil {
		return fmt.Errorf("DeleteVariant(%s) inventory: %w", id, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM variants WHERE id = $1`, id); err != nil {
		return fmt.Errorf("DeleteVariant(%s): %w", id, err)
	}
	return nil
}

// =============================================================================
// Inventory seeding
// =============================================================================

// InitInventory creates a zero-stock inventory row for a newly created variant.
// Must be called inside the same transaction as InsertVariant.
func (r *ProductRepository) InitInventory(ctx context.Context, tx pgx.Tx, variantID uuid.UUID) error {
	const q = `
		INSERT INTO inventory
		    (id, variant_id, quantity_on_hand, quantity_reserved,
		     reorder_point, reorder_qty, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, 0, 0, 10, 50,
		        NOW() AT TIME ZONE 'UTC', NOW() AT TIME ZONE 'UTC')`

	_, err := tx.Exec(ctx, q, variantID)
	if err != nil {
		return fmt.Errorf("InitInventory(variant=%s): %w", variantID, err)
	}
	return nil
}

// =============================================================================
// Channel Prices
// =============================================================================

// UpsertChannelPrice inserts or updates the price for a variant on a channel.
func (r *ProductRepository) UpsertChannelPrice(ctx context.Context, cp *domain.ChannelPrice) error {
	const q = `
		INSERT INTO channel_prices
		    (id, variant_id, channel_id, price, currency, is_active,
		     effective_from, effective_until, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,
		        NOW() AT TIME ZONE 'UTC', $7,
		        NOW() AT TIME ZONE 'UTC', NOW() AT TIME ZONE 'UTC')
		ON CONFLICT (variant_id, channel_id) DO UPDATE
		    SET price          = EXCLUDED.price,
		        currency       = EXCLUDED.currency,
		        is_active      = EXCLUDED.is_active,
		        effective_from = EXCLUDED.effective_from,
		        updated_at     = NOW() AT TIME ZONE 'UTC'`

	_, err := r.pool.Exec(ctx, q,
		cp.ID, cp.VariantID, cp.ChannelID, cp.Price,
		cp.Currency, cp.IsActive, cp.EffectiveUntil,
	)
	if err != nil {
		return fmt.Errorf("UpsertChannelPrice(variant=%s, channel=%s): %w",
			cp.VariantID, cp.ChannelID, err)
	}
	return nil
}

// GetChannelPrice fetches the active price for a variant on a specific channel.
func (r *ProductRepository) GetChannelPrice(
	ctx context.Context,
	variantID, channelID uuid.UUID,
) (*domain.ChannelPrice, error) {
	const q = `
		SELECT id, variant_id, channel_id, price, currency, is_active,
		       effective_from, effective_until, created_at, updated_at
		  FROM channel_prices
		 WHERE variant_id = $1
		   AND channel_id = $2
		   AND is_active  = TRUE
		   AND (effective_until IS NULL OR effective_until > NOW() AT TIME ZONE 'UTC')`

	cp := &domain.ChannelPrice{}
	err := r.pool.QueryRow(ctx, q, variantID, channelID).Scan(
		&cp.ID, &cp.VariantID, &cp.ChannelID, &cp.Price, &cp.Currency, &cp.IsActive,
		&cp.EffectiveFrom, &cp.EffectiveUntil, &cp.CreatedAt, &cp.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetChannelPrice(variant=%s, channel=%s): %w",
			variantID, channelID, err)
	}
	return cp, nil
}

// GetChannelByType looks up a channel ID by type string (used in order processing).
func (r *ProductRepository) GetChannelByType(ctx context.Context, channelType domain.ChannelType) (*domain.Channel, error) {
	const q = `
		SELECT id, name, type, is_active, description, created_at, updated_at
		  FROM channels
		 WHERE type = $1 AND is_active = TRUE
		 LIMIT 1`

	ch := &domain.Channel{}
	err := r.pool.QueryRow(ctx, q, channelType).Scan(
		&ch.ID, &ch.Name, &ch.Type, &ch.IsActive,
		&ch.Description, &ch.CreatedAt, &ch.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetChannelByType(%s): %w", channelType, err)
	}
	return ch, nil
}

// ListProducts returns paginated product/variant rows with stock and price.
func (r *ProductRepository) ListProducts(ctx context.Context, filters domain.ProductListFilters) ([]domain.ProductListItem, int, error) {
	if filters.Page < 1 {
		filters.Page = 1
	}
	if filters.PageSize < 1 || filters.PageSize > 100 {
		filters.PageSize = 25
	}
	offset := (filters.Page - 1) * filters.PageSize

	args := []any{}
	where := []string{"(p.name IS NOT NULL AND BTRIM(p.name) <> '')"}
	argIdx := 1
	if filters.Status == "" {
		where = append(where, "p.status <> 'archived'")
	}
	if filters.Search != "" {
		where = append(where, fmt.Sprintf(`(
			p.name ILIKE $%d OR EXISTS (
				SELECT 1
				FROM variants vx
				WHERE vx.product_id = p.id
				  AND vx.sku ILIKE $%d
			)
		)`, argIdx, argIdx))
		args = append(args, "%"+strings.TrimSpace(filters.Search)+"%")
		argIdx++
	}
	if filters.Status != "" {
		where = append(where, fmt.Sprintf("p.status = $%d", argIdx))
		args = append(args, filters.Status)
		argIdx++
	}
	if filters.Category != "" {
		where = append(where, fmt.Sprintf("p.category = $%d", argIdx))
		args = append(args, filters.Category)
		argIdx++
	}
	if filters.CollectionID != nil {
		where = append(where, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM product_collection_memberships pcm
			WHERE pcm.product_id = p.id AND pcm.collection_id = $%d)`, argIdx))
		args = append(args, *filters.CollectionID)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")

	countQ := fmt.Sprintf(`
		SELECT COUNT(*) FROM products p
		WHERE %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListProducts count: %w", err)
	}

	args = append(args, filters.PageSize, offset)
	limitIdx := argIdx
	offsetIdx := argIdx + 1
	stockExpr := `COALESCE(i.quantity_available, 0)`
	if filters.AggregateVariantInventory {
		stockExpr = `(SELECT COALESCE(SUM(COALESCE(i2.quantity_available, 0)), 0)
		                FROM variants v2
		                LEFT JOIN inventory i2 ON i2.variant_id = v2.id
		               WHERE v2.product_id = p.id AND v2.is_active = TRUE)`
	}
	listQ := fmt.Sprintf(`
		SELECT COALESCE(v.id, p.id) AS row_id,
		       p.id AS product_id,
		       COALESCE(p.name, '') AS name,
		       COALESCE(p.slug, '') AS slug,
		       COALESCE(v.sku, '') AS sku,
		       p.category,
		       v.image_url AS thumbnail,
		       COALESCE((SELECT cp.price FROM channel_prices cp
		                JOIN channels c ON c.id = cp.channel_id AND c.is_active = TRUE
		                WHERE cp.variant_id = v.id AND cp.is_active = TRUE
		                  AND (cp.effective_until IS NULL OR cp.effective_until > NOW())
		                ORDER BY cp.effective_from DESC NULLS LAST LIMIT 1), 0) AS price,
		       %s AS stock,
		       p.status,
		       p.created_at
		  FROM products p
		  LEFT JOIN LATERAL (
		      SELECT id, sku, image_url
		      FROM variants
		      WHERE product_id = p.id
		        AND is_active = TRUE
		      ORDER BY created_at DESC
		      LIMIT 1
		  ) v ON TRUE
		  LEFT JOIN inventory i ON i.variant_id = v.id
		 WHERE %s
		 ORDER BY p.created_at DESC
		 LIMIT $%d OFFSET $%d`, stockExpr, whereClause, limitIdx, offsetIdx)

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("ListProducts: %w", err)
	}
	defer rows.Close()

	var items []domain.ProductListItem
	for rows.Next() {
		var it domain.ProductListItem
		var price decimal.Decimal
		if err := rows.Scan(&it.ID, &it.ProductID, &it.Name, &it.Slug, &it.SKU, &it.Category, &it.Thumbnail,
			&price, &it.Stock, &it.Status, &it.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("ListProducts scan: %w", err)
		}
		it.Price = price
		items = append(items, it)
	}
	return items, total, rows.Err()
}
