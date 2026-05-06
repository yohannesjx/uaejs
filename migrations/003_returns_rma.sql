-- =============================================================================
-- Migration 003: Returns / RMA Module + Outbound QC Photos
-- =============================================================================

-- ── Enums ────────────────────────────────────────────────────────────────────

CREATE TYPE return_status AS ENUM (
    'pending',      -- created by customer, awaiting warehouse receipt
    'received',     -- physical item received at warehouse
    'qc_review',    -- QC photo comparison underway
    'approved',     -- return approved, stock/COGS adjusted
    'rejected',     -- return denied (fraud signal, outside policy, etc.)
    'completed'     -- credit/refund issued, audit trail closed
);

CREATE TYPE item_condition AS ENUM (
    'good',         -- resaleable, can be returned to inventory
    'damaged',      -- cannot be resold; written off as loss
    'wrong_item'    -- customer received wrong SKU; no quality assessment
);

-- ── Tables ───────────────────────────────────────────────────────────────────

CREATE TABLE returns (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id            UUID NOT NULL REFERENCES orders(id),
    status              return_status NOT NULL DEFAULT 'pending',
    customer_name       TEXT,
    customer_email      TEXT,
    return_reason       TEXT NOT NULL,
    rejection_reason    TEXT,

    -- Lifecycle timestamps
    requested_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    received_at         TIMESTAMPTZ,
    resolved_at         TIMESTAMPTZ,

    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_returns_order_id ON returns(order_id);
CREATE INDEX idx_returns_status   ON returns(status);

-- ─────────────────────────────────────────────────────────────────────────────
-- Individual line items within a return request
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE return_items (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    return_id               UUID NOT NULL REFERENCES returns(id),
    order_item_id           UUID NOT NULL REFERENCES order_items(id),
    variant_id              UUID NOT NULL REFERENCES variants(id),

    -- FIFO traceability: which batch the stock came from (needed for COGS reversal)
    batch_item_id           UUID REFERENCES batch_items(id),

    quantity                INTEGER NOT NULL CHECK (quantity > 0),
    condition               item_condition NOT NULL,

    -- QC photo hashes (SHA-256 of the raw file bytes)
    -- NULL means no photo was provided / not yet processed.
    qc_photo_hash_customer  TEXT,   -- hash of the photo submitted by the customer
    qc_photo_hash_outbound  TEXT,   -- hash stored at shipment time

    -- 1.0 = exact match (same photo), 0.0 = completely different
    qc_match_score          NUMERIC(5,4),
    qc_passed               BOOLEAN,
    qc_reviewed_at          TIMESTAMPTZ,
    qc_reviewer_notes       TEXT,

    -- Financial impact when approved
    -- Copied from order_items.cogs_per_unit at approval time for immutable audit.
    cogs_per_unit_reversed  NUMERIC(19,4),

    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_return_items_return_id  ON return_items(return_id);
CREATE INDEX idx_return_items_variant_id ON return_items(variant_id);

-- ─────────────────────────────────────────────────────────────────────────────
-- Photo asset registry – decoupled from return_items so photos can be added
-- incrementally and supports multiple photos per item (outbound + customer).
-- The actual file is stored in object storage; only the hash + path is here.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TYPE photo_type AS ENUM ('outbound_qc', 'customer_submitted', 'warehouse_received');

CREATE TABLE return_photos (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    return_item_id  UUID NOT NULL REFERENCES return_items(id),
    photo_type      photo_type NOT NULL,
    file_hash       TEXT NOT NULL,      -- SHA-256 hex of file bytes
    file_size_bytes BIGINT,
    mime_type       TEXT,               -- e.g. image/jpeg
    storage_path    TEXT NOT NULL,      -- e.g. s3://bucket/qc/2024/...
    uploaded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_return_photos_item_id ON return_photos(return_item_id);

-- ── Triggers ─────────────────────────────────────────────────────────────────
CREATE TRIGGER trg_returns_updated_at
    BEFORE UPDATE ON returns
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_return_items_updated_at
    BEFORE UPDATE ON return_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
