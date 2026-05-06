-- =============================================================================
-- Migration 009: POS System
--
-- Adds dedicated POS infrastructure: registers, cashier sessions, and
-- payment records. POS orders are standard orders (channel_type = 'pos')
-- processed through the existing order and inventory pipeline.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- POS Registers  (physical checkout terminals)
-- ---------------------------------------------------------------------------
CREATE TABLE pos_registers (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,           -- e.g. "Checkout 1"
    location    TEXT        NOT NULL DEFAULT '', -- e.g. "Ground Floor"
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE TRIGGER trg_pos_registers_updated_at
    BEFORE UPDATE ON pos_registers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- POS Sessions  (cashier shift = register opened → closed)
-- ---------------------------------------------------------------------------
CREATE TABLE pos_sessions (
    id           UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    register_id  UUID           NOT NULL REFERENCES pos_registers(id),
    opened_by    UUID           NOT NULL REFERENCES users(id),
    opened_at    TIMESTAMPTZ    NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    closed_at    TIMESTAMPTZ,
    opening_cash NUMERIC(19,4)  NOT NULL DEFAULT 0 CHECK (opening_cash >= 0),
    closing_cash NUMERIC(19,4)                     CHECK (closing_cash >= 0),
    notes        TEXT,
    created_at   TIMESTAMPTZ    NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_pos_sessions_register ON pos_sessions (register_id);
CREATE INDEX idx_pos_sessions_opened_by ON pos_sessions (opened_by);

-- ---------------------------------------------------------------------------
-- POS Payments  (one payment row per transaction; supports split payments)
-- ---------------------------------------------------------------------------
CREATE TYPE pos_payment_method AS ENUM ('cash', 'card', 'split');

CREATE TABLE pos_payments (
    id             UUID               PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id       UUID               NOT NULL REFERENCES orders(id),
    session_id     UUID               REFERENCES pos_sessions(id),
    payment_method pos_payment_method NOT NULL,
    amount         NUMERIC(19,4)      NOT NULL CHECK (amount > 0),
    currency       CHAR(3)            NOT NULL DEFAULT 'AED',
    reference      TEXT,   -- card terminal reference / cash drawer note
    paid_at        TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_pos_payments_order_id   ON pos_payments (order_id);
CREATE INDEX idx_pos_payments_session_id ON pos_payments (session_id);
CREATE INDEX idx_pos_payments_paid_at    ON pos_payments (paid_at DESC);

-- ---------------------------------------------------------------------------
-- Seed a default POS register so the system works out of the box
-- ---------------------------------------------------------------------------
INSERT INTO pos_registers (id, name, location)
VALUES ('00000000-0000-0000-0000-000000000010', 'Main Register', 'Store Floor')
ON CONFLICT DO NOTHING;
