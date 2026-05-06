-- 016_product_draft_status.sql
-- Add status enum and make product name & variant sku nullable for drafts

CREATE TYPE product_status AS ENUM ('draft', 'active', 'archived');

ALTER TABLE products ADD COLUMN status product_status NOT NULL DEFAULT 'draft';

-- Migrate existing Data (If any exist, they are active based on the previous boolean logic)
UPDATE products SET status = CASE WHEN is_active THEN 'active'::product_status ELSE 'archived'::product_status END;

-- Drop the old is_active column since status replaces it
ALTER TABLE products DROP COLUMN is_active;

-- Drop NOT NULL from products.name so drafts can be created empty
ALTER TABLE products ALTER COLUMN name DROP NOT NULL;

-- Drop NOT NULL from variants.sku so default variants can be created without SKUs immediately
ALTER TABLE variants ALTER COLUMN sku DROP NOT NULL;
