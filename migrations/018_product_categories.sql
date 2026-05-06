-- +goose Up
-- +goose StatementBegin

CREATE TABLE product_categories (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    type VARCHAR(50) NOT NULL CHECK (type IN ('manual', 'smart')),
    image_url TEXT,
    conditions JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for slug lookups and tenant queries
CREATE INDEX idx_product_categories_slug ON product_categories(slug);
CREATE INDEX idx_product_categories_tenant ON product_categories(tenant_id);

CREATE TABLE product_category_memberships (
    category_id UUID NOT NULL REFERENCES product_categories(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (category_id, product_id)
);

-- Down mapping
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS product_category_memberships;
DROP TABLE IF EXISTS product_categories;
-- +goose StatementEnd
