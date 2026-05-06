CREATE TABLE IF NOT EXISTS variant_media (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id UUID NOT NULL REFERENCES variants(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (variant_id, url)
);

CREATE INDEX IF NOT EXISTS idx_variant_media_variant_id
    ON variant_media (variant_id, sort_order, created_at);
