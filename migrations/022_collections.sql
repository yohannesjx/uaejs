CREATE TABLE IF NOT EXISTS product_collections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL,
    description TEXT,
    image_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_product_collections_tenant
    ON product_collections (tenant_id);

CREATE INDEX IF NOT EXISTS idx_product_collections_slug_lookup
    ON product_collections (tenant_id, slug);

CREATE TABLE IF NOT EXISTS product_collection_memberships (
    collection_id UUID NOT NULL REFERENCES product_collections(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (collection_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_pcm_product
    ON product_collection_memberships (product_id);

CREATE INDEX IF NOT EXISTS idx_pcm_collection
    ON product_collection_memberships (collection_id);
