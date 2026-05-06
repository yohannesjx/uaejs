package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureAdminSchema runs idempotent DDL for tables/columns used by the admin UI that
// may be missing when Postgres was initialized before those migrations existed, or when
// init scripts contained destructive goose "Down" sections.
//
// Safe to call on every process start (IF NOT EXISTS / ADD COLUMN IF NOT EXISTS).
func EnsureAdminSchema(ctx context.Context, pool *pgxpool.Pool) error {
	for i, q := range ensureAdminSchemaSQL {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("ensure admin schema stmt %d: %w", i, err)
		}
	}
	return nil
}

var ensureAdminSchemaSQL = []string{
	// 017 media_assets
	`CREATE TABLE IF NOT EXISTS media_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    url TEXT NOT NULL,
    mime_type VARCHAR(50) NOT NULL,
    size_bytes BIGINT NOT NULL,
    alt TEXT,
    tags TEXT[] DEFAULT '{}',
    sort_order INT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`,
	// 018 product_categories
	`CREATE TABLE IF NOT EXISTS product_categories (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(50) NOT NULL CHECK (type IN ('manual', 'smart')),
    image_url TEXT,
    conditions JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS uq_product_categories_slug_global ON product_categories(slug)`,
	`CREATE INDEX IF NOT EXISTS idx_product_categories_tenant ON product_categories(tenant_id)`,
	`CREATE TABLE IF NOT EXISTS product_category_memberships (
    category_id UUID NOT NULL REFERENCES product_categories(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (category_id, product_id)
)`,
	// 023 promotion efficacy column
	`ALTER TABLE order_items
    ADD COLUMN IF NOT EXISTS promotion_id UUID REFERENCES price_promotions(id) ON DELETE SET NULL`,
	`CREATE INDEX IF NOT EXISTS idx_order_items_promotion_id
    ON order_items (promotion_id)
    WHERE promotion_id IS NOT NULL`,
}
