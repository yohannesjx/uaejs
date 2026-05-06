-- =============================================================================
-- Migration 006: Supplier Module + Procurement
-- =============================================================================

CREATE TABLE suppliers (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL,
    contact_name      TEXT,
    phone             TEXT,
    email             TEXT,
    country           TEXT,
    lead_time_days    INT DEFAULT 30,
    minimum_order_qty INT DEFAULT 1,
    rating            INT CHECK (rating BETWEEN 1 AND 5),
    notes             TEXT,
    is_active         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_suppliers_name    ON suppliers(name);
CREATE INDEX idx_suppliers_country ON suppliers(country);

-- purchase_orders: one PO per supplier request
CREATE TYPE po_status AS ENUM ('draft', 'sent', 'confirmed', 'partially_received', 'received', 'cancelled');

CREATE TABLE purchase_orders (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_id      UUID REFERENCES suppliers(id),
    status           po_status NOT NULL DEFAULT 'draft',
    reference_number TEXT,
    notes            TEXT,
    total_cost       NUMERIC(19,4),
    currency         TEXT NOT NULL DEFAULT 'AED',
    expected_at      TIMESTAMPTZ,
    received_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_purchase_orders_supplier ON purchase_orders(supplier_id);
CREATE INDEX idx_purchase_orders_status   ON purchase_orders(status);

CREATE TABLE purchase_order_items (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    purchase_order_id UUID NOT NULL REFERENCES purchase_orders(id) ON DELETE CASCADE,
    variant_id        UUID NOT NULL REFERENCES variants(id),
    quantity          INT NOT NULL CHECK (quantity > 0),
    unit_cost         NUMERIC(19,4) NOT NULL CHECK (unit_cost > 0),
    received_qty      INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_po_items_po_id      ON purchase_order_items(purchase_order_id);
CREATE INDEX idx_po_items_variant_id ON purchase_order_items(variant_id);

-- supplier_id on purchase_batches (nullable – system works without suppliers)
ALTER TABLE purchase_batches
    ADD COLUMN IF NOT EXISTS supplier_id UUID REFERENCES suppliers(id);

CREATE INDEX IF NOT EXISTS idx_purchase_batches_supplier ON purchase_batches(supplier_id)
    WHERE supplier_id IS NOT NULL;

CREATE TRIGGER trg_suppliers_updated_at
    BEFORE UPDATE ON suppliers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_purchase_orders_updated_at
    BEFORE UPDATE ON purchase_orders
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
