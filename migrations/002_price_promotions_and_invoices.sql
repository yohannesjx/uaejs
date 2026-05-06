-- =============================================================================
-- Migration 002: Price Promotions, Order Invoices & Invoice Sequence
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Customer Tier enum – used for tier-based promotion targeting
-- ---------------------------------------------------------------------------
CREATE TYPE customer_tier AS ENUM ('standard', 'vip', 'wholesale', 'staff');

-- ---------------------------------------------------------------------------
-- Invoice number sequence
-- Format produced: INV-{YEAR}-{6-digit padded seq}  e.g. INV-2026-001042
-- ---------------------------------------------------------------------------
CREATE SEQUENCE invoice_number_seq
    START WITH 1000
    INCREMENT BY 1
    NO CYCLE;

-- ---------------------------------------------------------------------------
-- price_promotions
-- A time-bounded discount price for a specific variant + channel combination.
-- Optional customer_tier allows tier-targeted promotions (NULL = all tiers).
-- ---------------------------------------------------------------------------
CREATE TABLE price_promotions (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    variant_id      UUID          NOT NULL REFERENCES variants(id) ON DELETE CASCADE,
    channel_id      UUID          NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    customer_tier   customer_tier,          -- NULL = applies to all tiers
    promo_price     NUMERIC(19,4) NOT NULL CHECK (promo_price >= 0),
    currency        CHAR(3)       NOT NULL DEFAULT 'AED',
    effective_from  TIMESTAMPTZ   NOT NULL,
    effective_until TIMESTAMPTZ   NOT NULL,
    is_active       BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),

    CONSTRAINT chk_promo_dates CHECK (effective_until > effective_from)
);

CREATE INDEX idx_promotions_variant_channel ON price_promotions (variant_id, channel_id)
    WHERE is_active = TRUE;
CREATE INDEX idx_promotions_effective       ON price_promotions (effective_from, effective_until)
    WHERE is_active = TRUE;

CREATE TRIGGER trg_price_promotions_updated_at
    BEFORE UPDATE ON price_promotions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- order_invoices  (immutable; compliance archive)
-- Stores UBL XML for B2B/wholesale orders and receipt markers for retail.
-- ---------------------------------------------------------------------------
CREATE TYPE invoice_doc_type AS ENUM ('einvoice_ubl', 'receipt');

CREATE TABLE order_invoices (
    id              UUID              PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        UUID              NOT NULL REFERENCES orders(id),
    invoice_type    invoice_doc_type  NOT NULL,
    invoice_number  TEXT              NOT NULL UNIQUE,
    -- NULL for receipt; full UBL 2.1 XML string for e-invoice
    xml_content     TEXT,
    -- Exchange rate used when order currency ≠ AED (BT-111)
    exchange_rate_to_aed NUMERIC(12,6) NOT NULL DEFAULT 1.000000,
    trigger_reason  TEXT              NOT NULL, -- e.g. 'wholesale_order', 'b2b_trn', 'retail_receipt'
    issued_at       TIMESTAMPTZ       NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    created_at      TIMESTAMPTZ       NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_order_invoices_order_id ON order_invoices (order_id);
CREATE INDEX idx_order_invoices_number   ON order_invoices (invoice_number);
CREATE INDEX idx_order_invoices_issued   ON order_invoices (issued_at DESC);

-- ---------------------------------------------------------------------------
-- Helper function: generate the next invoice number
-- Called within the order transaction to guarantee uniqueness.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION next_invoice_number()
RETURNS TEXT LANGUAGE SQL AS $$
    SELECT 'INV-' || EXTRACT(YEAR FROM NOW() AT TIME ZONE 'UTC')::TEXT
        || '-' || LPAD(nextval('invoice_number_seq')::TEXT, 6, '0');
$$;
