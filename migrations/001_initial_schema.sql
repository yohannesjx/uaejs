-- =============================================================================
-- Dubai Fast-Fashion Retail & Wholesale OS
-- PostgreSQL 16 · Initial Schema Migration
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Extensions
-- ---------------------------------------------------------------------------
CREATE EXTENSION IF NOT EXISTS "pgcrypto";   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS "pg_trgm";    -- trigram search on product names

-- ---------------------------------------------------------------------------
-- Enums
-- ---------------------------------------------------------------------------
CREATE TYPE channel_type AS ENUM ('pos', 'ecommerce', 'wholesale');
CREATE TYPE movement_type AS ENUM (
    'purchase_in',
    'sale_out',
    'adjustment_in',
    'adjustment_out',
    'reservation',
    'reservation_release',
    'return_in'
);
CREATE TYPE order_status AS ENUM (
    'pending',
    'reserved',
    'confirmed',
    'shipped',
    'completed',
    'cancelled',
    'refunded'
);
CREATE TYPE payment_status AS ENUM ('unpaid', 'partial', 'paid', 'refunded');
CREATE TYPE vat_type AS ENUM ('standard', 'zero_rated', 'exempt');

-- ---------------------------------------------------------------------------
-- Utility: auto-update updated_at
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = NOW() AT TIME ZONE 'UTC';
    RETURN NEW;
END;
$$;

-- =============================================================================
-- 1. PRODUCTS
-- =============================================================================
CREATE TABLE products (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT        NOT NULL,
    name_ar         TEXT,                           -- Arabic name for UAE invoicing
    description     TEXT,
    brand           TEXT,
    category        TEXT,
    sub_category    TEXT,
    is_active       BOOLEAN     NOT NULL DEFAULT TRUE,
    vat_type        vat_type    NOT NULL DEFAULT 'standard',
    hs_code         TEXT,                           -- Harmonised System code (customs)
    country_of_origin TEXT      NOT NULL DEFAULT 'CN',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_products_name_trgm ON products USING gin (name gin_trgm_ops);
CREATE INDEX idx_products_category  ON products (category, sub_category);
CREATE INDEX idx_products_is_active ON products (is_active);

CREATE TRIGGER trg_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- 2. VARIANTS  (size / color / SKU)
-- =============================================================================
CREATE TABLE variants (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id  UUID        NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    sku         TEXT        NOT NULL UNIQUE,
    barcode     TEXT        UNIQUE,
    color       TEXT,
    size        TEXT,
    weight_g    NUMERIC(10,3),                      -- grams – used in shipping calc
    image_url   TEXT,
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_variants_product_id ON variants (product_id);
CREATE INDEX idx_variants_sku        ON variants (sku);
CREATE INDEX idx_variants_barcode    ON variants (barcode) WHERE barcode IS NOT NULL;

CREATE TRIGGER trg_variants_updated_at
    BEFORE UPDATE ON variants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- 3. CHANNELS
-- =============================================================================
CREATE TABLE channels (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT         NOT NULL UNIQUE,
    type         channel_type NOT NULL,
    is_active    BOOLEAN      NOT NULL DEFAULT TRUE,
    description  TEXT,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE TRIGGER trg_channels_updated_at
    BEFORE UPDATE ON channels
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Seed default channels
INSERT INTO channels (name, type) VALUES
    ('POS Dubai Store',   'pos'),
    ('Website',           'ecommerce'),
    ('Wholesale',         'wholesale');

-- =============================================================================
-- 4. CHANNEL PRICES
-- =============================================================================
CREATE TABLE channel_prices (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id      UUID        NOT NULL REFERENCES variants(id) ON DELETE CASCADE,
    channel_id      UUID        NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    price           NUMERIC(19,4) NOT NULL CHECK (price >= 0),
    currency        CHAR(3)     NOT NULL DEFAULT 'AED',
    is_active       BOOLEAN     NOT NULL DEFAULT TRUE,
    effective_from  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    effective_until TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    UNIQUE (variant_id, channel_id)
);

CREATE INDEX idx_channel_prices_variant  ON channel_prices (variant_id);
CREATE INDEX idx_channel_prices_channel  ON channel_prices (channel_id);

CREATE TRIGGER trg_channel_prices_updated_at
    BEFORE UPDATE ON channel_prices
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- 5. PURCHASE BATCHES  (import shipments from China)
-- =============================================================================
CREATE TABLE purchase_batches (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    reference           TEXT        NOT NULL UNIQUE,        -- e.g. PO-2026-001
    supplier_name       TEXT        NOT NULL,
    origin_country      CHAR(2)     NOT NULL DEFAULT 'CN',

    -- Shipment-level costs (AED)
    total_shipping_cost NUMERIC(19,4) NOT NULL DEFAULT 0 CHECK (total_shipping_cost >= 0),
    total_insurance     NUMERIC(19,4) NOT NULL DEFAULT 0 CHECK (total_insurance >= 0),
    customs_duty_rate   NUMERIC(6,4) NOT NULL DEFAULT 0.05, -- 5% UAE customs

    -- Dates
    ordered_at          TIMESTAMPTZ NOT NULL,
    received_at         TIMESTAMPTZ,

    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_purchase_batches_received ON purchase_batches (received_at DESC NULLS LAST);

CREATE TRIGGER trg_purchase_batches_updated_at
    BEFORE UPDATE ON purchase_batches
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- 6. BATCH ITEMS  (line items within a purchase batch)
--    Landed cost is computed and stored per unit for FIFO integrity.
-- =============================================================================
CREATE TABLE batch_items (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id            UUID        NOT NULL REFERENCES purchase_batches(id) ON DELETE CASCADE,
    variant_id          UUID        NOT NULL REFERENCES variants(id),

    quantity_ordered    INTEGER     NOT NULL CHECK (quantity_ordered > 0),
    quantity_received   INTEGER     NOT NULL DEFAULT 0 CHECK (quantity_received >= 0),

    -- Per-unit costs (AED)
    unit_cost           NUMERIC(19,4) NOT NULL CHECK (unit_cost >= 0),
    shipping_allocation NUMERIC(19,4) NOT NULL DEFAULT 0 CHECK (shipping_allocation >= 0),
    customs_duty        NUMERIC(19,4) NOT NULL DEFAULT 0 CHECK (customs_duty >= 0),
    insurance           NUMERIC(19,4) NOT NULL DEFAULT 0 CHECK (insurance >= 0),

    -- Computed and stored for immutable FIFO reference
    -- landed_cost = unit_cost + shipping_allocation + customs_duty + insurance
    landed_cost_per_unit NUMERIC(19,4) GENERATED ALWAYS AS (
        unit_cost + shipping_allocation + customs_duty + insurance
    ) STORED,

    -- Input VAT on import (5%)
    input_vat_per_unit  NUMERIC(19,4) NOT NULL DEFAULT 0,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_batch_items_batch_id   ON batch_items (batch_id);
CREATE INDEX idx_batch_items_variant_id ON batch_items (variant_id);

CREATE TRIGGER trg_batch_items_updated_at
    BEFORE UPDATE ON batch_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- 7. INVENTORY  (single global pool per variant)
-- =============================================================================
CREATE TABLE inventory (
    id                  UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id          UUID    NOT NULL REFERENCES variants(id) UNIQUE,

    -- Quantities
    quantity_on_hand    INTEGER NOT NULL DEFAULT 0 CHECK (quantity_on_hand >= 0),
    quantity_reserved   INTEGER NOT NULL DEFAULT 0 CHECK (quantity_reserved >= 0),
    quantity_available  INTEGER GENERATED ALWAYS AS (quantity_on_hand - quantity_reserved) STORED,

    reorder_point       INTEGER NOT NULL DEFAULT 10,
    reorder_qty         INTEGER NOT NULL DEFAULT 50,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_inventory_variant_id ON inventory (variant_id);
CREATE INDEX idx_inventory_low_stock  ON inventory (quantity_available) WHERE quantity_available <= 10;

CREATE TRIGGER trg_inventory_updated_at
    BEFORE UPDATE ON inventory
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- 8. INVENTORY MOVEMENTS  (immutable ledger)
-- =============================================================================
CREATE TABLE inventory_movements (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id      UUID            NOT NULL REFERENCES variants(id),
    batch_item_id   UUID            REFERENCES batch_items(id),    -- nullable for non-purchase moves
    order_id        UUID,                                          -- FK added after orders table
    reservation_id  UUID,                                          -- FK added after reservations table

    movement_type   movement_type   NOT NULL,
    quantity        INTEGER         NOT NULL,                       -- positive = in, negative = out
    quantity_before INTEGER         NOT NULL,
    quantity_after  INTEGER         NOT NULL,

    -- Cost snapshot at time of movement (for COGS)
    unit_cost_snapshot NUMERIC(19,4),

    channel_id      UUID            REFERENCES channels(id),
    reference       TEXT,                                          -- free-text reference
    notes           TEXT,

    -- Immutable: no updated_at
    created_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),

    -- Prevent mutation
    CONSTRAINT chk_movement_qty_nonzero CHECK (quantity != 0)
);

CREATE INDEX idx_inv_movements_variant    ON inventory_movements (variant_id, created_at DESC);
CREATE INDEX idx_inv_movements_order      ON inventory_movements (order_id) WHERE order_id IS NOT NULL;
CREATE INDEX idx_inv_movements_batch_item ON inventory_movements (batch_item_id) WHERE batch_item_id IS NOT NULL;
CREATE INDEX idx_inv_movements_type       ON inventory_movements (movement_type, created_at DESC);

-- =============================================================================
-- 9. ORDERS
-- =============================================================================
CREATE TABLE orders (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id      UUID            NOT NULL REFERENCES channels(id),

    -- Customer info (denormalised for invoice compliance)
    customer_name   TEXT,
    customer_email  TEXT,
    customer_phone  TEXT,
    customer_trn    TEXT,           -- UAE Tax Registration Number (B2B)
    shipping_address JSONB,

    -- Financials (AED)
    subtotal        NUMERIC(19,4)   NOT NULL DEFAULT 0,
    discount_amount NUMERIC(19,4)   NOT NULL DEFAULT 0,
    vat_amount      NUMERIC(19,4)   NOT NULL DEFAULT 0,
    total_amount    NUMERIC(19,4)   NOT NULL DEFAULT 0,
    currency        CHAR(3)         NOT NULL DEFAULT 'AED',

    -- VAT / E-invoice
    vat_type        vat_type        NOT NULL DEFAULT 'standard',
    invoice_number  TEXT            UNIQUE,
    invoice_issued_at TIMESTAMPTZ,

    -- Statuses
    status          order_status    NOT NULL DEFAULT 'pending',
    payment_status  payment_status  NOT NULL DEFAULT 'unpaid',

    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_orders_channel    ON orders (channel_id, created_at DESC);
CREATE INDEX idx_orders_status     ON orders (status);
CREATE INDEX idx_orders_created_at ON orders (created_at DESC);
CREATE INDEX idx_orders_invoice    ON orders (invoice_number) WHERE invoice_number IS NOT NULL;

CREATE TRIGGER trg_orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- 10. ORDER ITEMS
-- =============================================================================
CREATE TABLE order_items (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        UUID        NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    variant_id      UUID        NOT NULL REFERENCES variants(id),

    quantity        INTEGER     NOT NULL CHECK (quantity > 0),
    unit_price      NUMERIC(19,4) NOT NULL CHECK (unit_price >= 0),
    discount_amount NUMERIC(19,4) NOT NULL DEFAULT 0,
    vat_rate        NUMERIC(6,4)  NOT NULL DEFAULT 0.05,   -- 0.00 for exports
    vat_amount      NUMERIC(19,4) NOT NULL DEFAULT 0,
    line_total      NUMERIC(19,4) NOT NULL DEFAULT 0,

    -- COGS snapshot at time of sale (FIFO)
    cogs_per_unit   NUMERIC(19,4),
    total_cogs      NUMERIC(19,4),

    created_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_order_items_order_id   ON order_items (order_id);
CREATE INDEX idx_order_items_variant_id ON order_items (variant_id);

CREATE TRIGGER trg_order_items_updated_at
    BEFORE UPDATE ON order_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- 11. STOCK RESERVATIONS
-- =============================================================================
CREATE TABLE stock_reservations (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    UUID        NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    variant_id  UUID        NOT NULL REFERENCES variants(id),
    quantity    INTEGER     NOT NULL CHECK (quantity > 0),

    expires_at  TIMESTAMPTZ NOT NULL,
    released_at TIMESTAMPTZ,                            -- NULL = still active
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_reservations_order_id    ON stock_reservations (order_id);
CREATE INDEX idx_reservations_variant_id  ON stock_reservations (variant_id) WHERE is_active = TRUE;
CREATE INDEX idx_reservations_expires_at  ON stock_reservations (expires_at) WHERE is_active = TRUE;

CREATE TRIGGER trg_reservations_updated_at
    BEFORE UPDATE ON stock_reservations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- DEFERRED FOREIGN KEY BACK-PATCHES
-- =============================================================================
ALTER TABLE inventory_movements
    ADD CONSTRAINT fk_inv_movements_order
    FOREIGN KEY (order_id) REFERENCES orders(id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE inventory_movements
    ADD CONSTRAINT fk_inv_movements_reservation
    FOREIGN KEY (reservation_id) REFERENCES stock_reservations(id) DEFERRABLE INITIALLY DEFERRED;

-- =============================================================================
-- VAT RATES REFERENCE TABLE  (UAE 2026 PINT-AE E-Invoice compliant)
-- =============================================================================
CREATE TABLE vat_categories (
    code        CHAR(1)     PRIMARY KEY,    -- S=Standard, Z=Zero, E=Exempt
    description TEXT        NOT NULL,
    rate        NUMERIC(6,4) NOT NULL,
    ubl_code    TEXT        NOT NULL        -- UN/CEFACT tax category code
);

INSERT INTO vat_categories VALUES
    ('S', 'Standard Rate',          0.05, 'S'),
    ('Z', 'Zero-Rated (Export)',     0.00, 'Z'),
    ('E', 'Exempt',                  0.00, 'E');

-- =============================================================================
-- AUDIT / SYSTEM LOG  (schema changes, critical operations)
-- =============================================================================
CREATE TABLE audit_log (
    id          BIGSERIAL   PRIMARY KEY,
    table_name  TEXT        NOT NULL,
    record_id   UUID        NOT NULL,
    operation   TEXT        NOT NULL,       -- INSERT | UPDATE | DELETE
    old_data    JSONB,
    new_data    JSONB,
    performed_by TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_audit_log_record  ON audit_log (table_name, record_id);
CREATE INDEX idx_audit_log_created ON audit_log (created_at DESC);
