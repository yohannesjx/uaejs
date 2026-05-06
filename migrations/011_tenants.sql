-- =============================================================================
-- Migration 011: SaaS Multi-Tenant Architecture
--
-- Introduces a tenants table and tenant_id columns on core business tables.
-- Single-store deployments use the default tenant automatically; no code
-- changes are required unless you want cross-tenant isolation at query level.
--
-- Default tenant UUID: 00000000-0000-0000-0000-000000000001
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Default tenant UUID constant used throughout this migration
-- ---------------------------------------------------------------------------
DO $$ BEGIN
    RAISE NOTICE 'Applying migration 011 — SaaS tenant architecture';
END $$;

-- ---------------------------------------------------------------------------
-- Tenants table
-- ---------------------------------------------------------------------------
CREATE TABLE tenants (
    id         UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT    NOT NULL,
    domain     TEXT    UNIQUE,           -- e.g. "store.myapp.com" or subdomain key
    plan       TEXT    NOT NULL DEFAULT 'starter',  -- starter / pro / enterprise
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_tenants_domain ON tenants (domain) WHERE domain IS NOT NULL;

CREATE TRIGGER trg_tenants_updated_at
    BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Seed the default "single-store" tenant used by all pre-existing data.
INSERT INTO tenants (id, name, domain, plan)
VALUES ('00000000-0000-0000-0000-000000000001', 'Default Store', NULL, 'enterprise')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- Tenant ↔ User mapping  (a user may belong to multiple tenants)
-- ---------------------------------------------------------------------------
CREATE TABLE tenant_users (
    tenant_id  UUID    NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id    UUID    NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    role       TEXT    NOT NULL DEFAULT 'member',  -- owner / admin / member
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    PRIMARY KEY (tenant_id, user_id)
);

CREATE INDEX idx_tenant_users_user ON tenant_users (user_id);

-- ---------------------------------------------------------------------------
-- Tenant Settings  (arbitrary JSONB config per tenant)
-- ---------------------------------------------------------------------------
CREATE TABLE tenant_settings (
    tenant_id  UUID    PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    settings   JSONB   NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

-- Seed default settings row for the default tenant.
INSERT INTO tenant_settings (tenant_id, settings)
VALUES ('00000000-0000-0000-0000-000000000001', '{"currency":"AED","vat_rate":"0.05","timezone":"Asia/Dubai"}')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- Add tenant_id to core business tables
-- Each column defaults to the default tenant UUID so existing rows are
-- automatically assigned and backward compatibility is preserved.
-- ---------------------------------------------------------------------------

-- Products
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL
        DEFAULT '00000000-0000-0000-0000-000000000001'
        REFERENCES tenants(id);
CREATE INDEX IF NOT EXISTS idx_products_tenant ON products (tenant_id);

-- Variants (inherits tenant via product but indexed for direct queries)
ALTER TABLE variants
    ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL
        DEFAULT '00000000-0000-0000-0000-000000000001'
        REFERENCES tenants(id);
CREATE INDEX IF NOT EXISTS idx_variants_tenant ON variants (tenant_id);

-- Orders
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL
        DEFAULT '00000000-0000-0000-0000-000000000001'
        REFERENCES tenants(id);
CREATE INDEX IF NOT EXISTS idx_orders_tenant ON orders (tenant_id);

-- Inventory
ALTER TABLE inventory
    ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL
        DEFAULT '00000000-0000-0000-0000-000000000001'
        REFERENCES tenants(id);
CREATE INDEX IF NOT EXISTS idx_inventory_tenant ON inventory (tenant_id);

-- Suppliers
ALTER TABLE suppliers
    ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL
        DEFAULT '00000000-0000-0000-0000-000000000001'
        REFERENCES tenants(id);
CREATE INDEX IF NOT EXISTS idx_suppliers_tenant ON suppliers (tenant_id);

-- Purchase Orders
ALTER TABLE purchase_orders
    ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL
        DEFAULT '00000000-0000-0000-0000-000000000001'
        REFERENCES tenants(id);
CREATE INDEX IF NOT EXISTS idx_purchase_orders_tenant ON purchase_orders (tenant_id);

COMMENT ON COLUMN products.tenant_id        IS 'Owning tenant. Defaults to the single-store tenant for backward compatibility.';
COMMENT ON COLUMN variants.tenant_id        IS 'Owning tenant. Inherited from the parent product.';
COMMENT ON COLUMN orders.tenant_id          IS 'Owning tenant. All order queries should filter by this column.';
COMMENT ON COLUMN inventory.tenant_id       IS 'Owning tenant. Each tenant maintains its own inventory pool.';
COMMENT ON COLUMN suppliers.tenant_id       IS 'Owning tenant. Suppliers are not shared across tenants.';
COMMENT ON COLUMN purchase_orders.tenant_id IS 'Owning tenant.';
