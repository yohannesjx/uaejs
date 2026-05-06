-- =============================================================================
-- Migration 012: Multi-Warehouse / Location Management
--
-- Adds per-location stock tracking. The existing global `inventory` table
-- remains the authoritative aggregate (no breaking changes). Warehouse stock
-- is an additive layer: tenants that don't configure warehouses see zero side
-- effects on all existing flows.
-- =============================================================================

-- Extend movement_type enum with warehouse transfer variants.
ALTER TYPE movement_type ADD VALUE IF NOT EXISTS 'transfer_in';
ALTER TYPE movement_type ADD VALUE IF NOT EXISTS 'transfer_out';

-- ---------------------------------------------------------------------------
-- Warehouse types
-- ---------------------------------------------------------------------------
CREATE TYPE warehouse_type AS ENUM ('warehouse', 'store', 'dropship', 'virtual');

-- ---------------------------------------------------------------------------
-- Warehouses  (physical or logical stock locations)
-- ---------------------------------------------------------------------------
CREATE TABLE warehouses (
    id          UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID            NOT NULL
                                    DEFAULT '00000000-0000-0000-0000-000000000001'
                                    REFERENCES tenants(id),
    name        TEXT            NOT NULL,
    type        warehouse_type  NOT NULL DEFAULT 'warehouse',
    address     TEXT            NOT NULL DEFAULT '',
    city        TEXT            NOT NULL DEFAULT '',
    country     CHAR(2)         NOT NULL DEFAULT 'AE',
    is_active   BOOLEAN         NOT NULL DEFAULT TRUE,
    priority    INT             NOT NULL DEFAULT 100, -- lower = higher fulfillment priority
    created_at  TIMESTAMPTZ     NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at  TIMESTAMPTZ     NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_warehouses_tenant    ON warehouses (tenant_id);
CREATE INDEX idx_warehouses_priority  ON warehouses (tenant_id, priority) WHERE is_active = TRUE;

CREATE TRIGGER trg_warehouses_updated_at
    BEFORE UPDATE ON warehouses
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Warehouse Stock  (per-location inventory ledger)
--
-- The global `inventory` table tracks aggregate on-hand + reserved totals.
-- `warehouse_stock` breaks that down by location. The sum of all
-- warehouse_stock.qty_on_hand for a variant should equal inventory.quantity_on_hand.
-- ---------------------------------------------------------------------------
CREATE TABLE warehouse_stock (
    id                  UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    warehouse_id        UUID    NOT NULL REFERENCES warehouses(id),
    variant_id          UUID    NOT NULL REFERENCES variants(id),
    qty_on_hand         INT     NOT NULL DEFAULT 0 CHECK (qty_on_hand >= 0),
    qty_reserved        INT     NOT NULL DEFAULT 0 CHECK (qty_reserved >= 0),
    qty_available       INT     GENERATED ALWAYS AS (qty_on_hand - qty_reserved) STORED,
    reorder_point       INT     NOT NULL DEFAULT 0,
    reorder_qty         INT     NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    UNIQUE (warehouse_id, variant_id)
);

CREATE INDEX idx_warehouse_stock_warehouse ON warehouse_stock (warehouse_id);
CREATE INDEX idx_warehouse_stock_variant   ON warehouse_stock (variant_id);
CREATE INDEX idx_warehouse_stock_available ON warehouse_stock (warehouse_id, qty_available) WHERE qty_available > 0;

CREATE TRIGGER trg_warehouse_stock_updated_at
    BEFORE UPDATE ON warehouse_stock
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Seed a default warehouse for single-store deployments.
-- ---------------------------------------------------------------------------
INSERT INTO warehouses (id, tenant_id, name, type, city, country, priority)
VALUES (
    '00000000-0000-0000-0000-000000000030',
    '00000000-0000-0000-0000-000000000001',
    'Main Warehouse',
    'warehouse',
    'Dubai',
    'AE',
    10
) ON CONFLICT DO NOTHING;

COMMENT ON TABLE warehouse_stock IS
    'Per-location stock counters. Sum of qty_on_hand = inventory.quantity_on_hand for each variant.';
COMMENT ON COLUMN warehouse_stock.qty_available IS
    'Computed: qty_on_hand - qty_reserved. Rows with qty_available > 0 are fulfillable from this location.';
