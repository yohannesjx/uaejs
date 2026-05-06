-- =============================================================================
-- Migration 010: Shipping / Fulfillment
--
-- Provider registry, account credentials, shipment records, and a tracking
-- event log. Connector adapters (Aramex, DHL, Emirates Post) implement the
-- ShippingConnector interface at the application layer.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Shipping Providers  (immutable registry of supported carriers)
-- ---------------------------------------------------------------------------
CREATE TABLE shipping_providers (
    id         UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT    NOT NULL UNIQUE,  -- "Aramex", "DHL", "Emirates Post"
    type       TEXT    NOT NULL UNIQUE,  -- connector key: "aramex", "dhl", "emiratespost"
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

INSERT INTO shipping_providers (id, name, type, is_active) VALUES
    ('00000000-0000-0000-0000-000000000020', 'Aramex',        'aramex',       TRUE),
    ('00000000-0000-0000-0000-000000000021', 'DHL',           'dhl',          FALSE),
    ('00000000-0000-0000-0000-000000000022', 'Emirates Post', 'emiratespost', FALSE)
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- Shipping Accounts  (store-specific API credentials per provider)
-- ---------------------------------------------------------------------------
CREATE TABLE shipping_accounts (
    id          UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID    NOT NULL REFERENCES shipping_providers(id),
    label       TEXT    NOT NULL DEFAULT '',   -- "Production", "Sandbox"
    api_key     TEXT    NOT NULL DEFAULT '',
    api_secret  TEXT    NOT NULL DEFAULT '',
    settings    JSONB   NOT NULL DEFAULT '{}',  -- provider-specific config
    is_active   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_shipping_accounts_provider ON shipping_accounts (provider_id);

CREATE TRIGGER trg_shipping_accounts_updated_at
    BEFORE UPDATE ON shipping_accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Shipments  (one shipment per fulfillable order)
-- ---------------------------------------------------------------------------
CREATE TYPE shipment_status AS ENUM (
    'pending',
    'booked',
    'picked_up',
    'in_transit',
    'out_for_delivery',
    'delivered',
    'failed',
    'cancelled',
    'returned'
);

CREATE TABLE shipments (
    id               UUID             PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id         UUID             NOT NULL REFERENCES orders(id),
    account_id       UUID             REFERENCES shipping_accounts(id),
    tracking_number  TEXT,
    carrier_ref      TEXT,            -- provider-specific booking reference
    status           shipment_status  NOT NULL DEFAULT 'pending',
    weight_g         NUMERIC(10,3),
    dimensions       JSONB,           -- {length_cm, width_cm, height_cm}
    created_at       TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_shipments_order_id        ON shipments (order_id);
CREATE INDEX idx_shipments_account_id      ON shipments (account_id);
CREATE INDEX idx_shipments_tracking_number ON shipments (tracking_number) WHERE tracking_number IS NOT NULL;
CREATE INDEX idx_shipments_status          ON shipments (status);

CREATE TRIGGER trg_shipments_updated_at
    BEFORE UPDATE ON shipments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Shipment Events  (immutable tracking event log)
-- ---------------------------------------------------------------------------
CREATE TABLE shipment_events (
    id          UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    shipment_id UUID    NOT NULL REFERENCES shipments(id) ON DELETE CASCADE,
    status      TEXT    NOT NULL,
    location    TEXT    NOT NULL DEFAULT '',
    description TEXT    NOT NULL DEFAULT '',
    event_time  TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_shipment_events_shipment  ON shipment_events (shipment_id, event_time DESC);
