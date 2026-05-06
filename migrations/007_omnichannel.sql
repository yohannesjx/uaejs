-- =============================================================================
-- Migration 007: Omnichannel Sync
-- Note: "channels" table already exists for internal POS/Web/Wholesale channels.
-- External marketplace connectors use "external_platforms" / "platform_accounts".
-- =============================================================================

CREATE TABLE external_platforms (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,                   -- e.g. "Shopify Store #1"
    type       TEXT NOT NULL,                   -- shopify|amazon|instagram|tiktok|noon
    is_active  BOOLEAN NOT NULL DEFAULT FALSE,  -- disabled until explicitly enabled
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_platforms_type ON external_platforms(type);

CREATE TABLE platform_accounts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    platform_id  UUID NOT NULL REFERENCES external_platforms(id) ON DELETE CASCADE,
    store_name   TEXT,
    api_key      TEXT NOT NULL,
    api_secret   TEXT NOT NULL,
    settings     JSONB NOT NULL DEFAULT '{}',   -- platform-specific config
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_platform_accounts_platform ON platform_accounts(platform_id);

-- Maps local variant → external platform listing
CREATE TABLE platform_products (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    platform_account_id   UUID NOT NULL REFERENCES platform_accounts(id) ON DELETE CASCADE,
    variant_id            UUID NOT NULL REFERENCES variants(id),
    external_product_id   TEXT NOT NULL,
    external_variant_id   TEXT,
    last_synced_at        TIMESTAMPTZ,
    sync_status           TEXT NOT NULL DEFAULT 'pending',  -- pending|synced|error
    sync_error            TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (platform_account_id, variant_id)
);

CREATE INDEX idx_platform_products_variant  ON platform_products(variant_id);
CREATE INDEX idx_platform_products_account  ON platform_products(platform_account_id);

-- Records orders imported from external platforms
CREATE TABLE platform_orders (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    platform_account_id  UUID NOT NULL REFERENCES platform_accounts(id),
    external_order_id    TEXT NOT NULL,
    local_order_id       UUID REFERENCES orders(id),
    status               TEXT NOT NULL DEFAULT 'pending',   -- pending|imported|failed|ignored
    raw_payload          JSONB,
    error_message        TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (platform_account_id, external_order_id)
);

CREATE INDEX idx_platform_orders_account       ON platform_orders(platform_account_id);
CREATE INDEX idx_platform_orders_local_order   ON platform_orders(local_order_id) WHERE local_order_id IS NOT NULL;
CREATE INDEX idx_platform_orders_status        ON platform_orders(status);

CREATE TRIGGER trg_platform_accounts_updated_at
    BEFORE UPDATE ON platform_accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_platform_orders_updated_at
    BEFORE UPDATE ON platform_orders
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
