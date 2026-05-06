-- =============================================================================
-- Migration 019: Add unique slug support to products
-- =============================================================================

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS slug TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_products_slug
    ON products(slug)
    WHERE slug IS NOT NULL;
