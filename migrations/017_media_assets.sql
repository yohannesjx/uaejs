-- +goose Up
CREATE TABLE IF NOT EXISTS media_assets (
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
);

-- +goose Down
DROP TABLE IF EXISTS media_assets;
