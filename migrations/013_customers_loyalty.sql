-- =============================================================================
-- Migration 013: Customer & Loyalty Module
--
-- Adds a customer registry, loyalty point accounts, and a full audit log of
-- point transactions. A nullable customer_id is added to orders for linkage.
-- All tables are tenant-scoped. Tenants that do not use this module see no
-- performance impact.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Loyalty tier enum  (separate from the pricing CustomerTier)
-- ---------------------------------------------------------------------------
CREATE TYPE loyalty_tier AS ENUM ('bronze', 'silver', 'gold', 'vip');

-- ---------------------------------------------------------------------------
-- Customers
-- ---------------------------------------------------------------------------
CREATE TABLE customers (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID         NOT NULL
                                  DEFAULT '00000000-0000-0000-0000-000000000001'
                                  REFERENCES tenants(id),
    email        TEXT         NOT NULL,
    phone        TEXT,
    full_name    TEXT         NOT NULL DEFAULT '',
    loyalty_tier loyalty_tier NOT NULL DEFAULT 'bronze',
    is_active    BOOLEAN      NOT NULL DEFAULT TRUE,
    notes        TEXT,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    UNIQUE (tenant_id, email)
);

CREATE INDEX idx_customers_tenant        ON customers (tenant_id);
CREATE INDEX idx_customers_email         ON customers (tenant_id, email);
CREATE INDEX idx_customers_phone         ON customers (tenant_id, phone) WHERE phone IS NOT NULL;
CREATE INDEX idx_customers_loyalty_tier  ON customers (tenant_id, loyalty_tier);

CREATE TRIGGER trg_customers_updated_at
    BEFORE UPDATE ON customers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Loyalty Accounts  (points balance per customer)
-- ---------------------------------------------------------------------------
CREATE TABLE loyalty_accounts (
    id              UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id     UUID    NOT NULL REFERENCES customers(id) UNIQUE,
    points_balance  INT     NOT NULL DEFAULT 0 CHECK (points_balance >= 0),
    lifetime_points INT     NOT NULL DEFAULT 0 CHECK (lifetime_points >= 0),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_loyalty_accounts_customer ON loyalty_accounts (customer_id);

-- ---------------------------------------------------------------------------
-- Loyalty Transactions  (immutable audit log)
-- ---------------------------------------------------------------------------
CREATE TYPE loyalty_tx_type AS ENUM ('earned', 'redeemed', 'expired', 'adjusted', 'refunded');

CREATE TABLE loyalty_transactions (
    id          UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id  UUID            NOT NULL REFERENCES loyalty_accounts(id),
    order_id    UUID            REFERENCES orders(id),
    tx_type     loyalty_tx_type NOT NULL,
    points      INT             NOT NULL,   -- positive = earned/adjusted, negative = redeemed
    balance_before INT          NOT NULL,
    balance_after  INT          NOT NULL,
    note        TEXT,
    created_at  TIMESTAMPTZ     NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_loyalty_tx_account  ON loyalty_transactions (account_id, created_at DESC);
CREATE INDEX idx_loyalty_tx_order    ON loyalty_transactions (order_id) WHERE order_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Add customer_id to orders (nullable for backward compatibility)
-- ---------------------------------------------------------------------------
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS customer_id UUID REFERENCES customers(id);

CREATE INDEX IF NOT EXISTS idx_orders_customer ON orders (customer_id) WHERE customer_id IS NOT NULL;

COMMENT ON COLUMN orders.customer_id IS 'Optional link to the customers registry. NULL = guest checkout.';
COMMENT ON TABLE loyalty_transactions IS 'Immutable ledger of every points event. Never UPDATE or DELETE rows.';
