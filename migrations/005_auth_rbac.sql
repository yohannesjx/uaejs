-- =============================================================================
-- Migration 005: Authentication + RBAC
-- =============================================================================

-- ── Users ─────────────────────────────────────────────────────────────────────
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    full_name     TEXT,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_email ON users(email);

-- ── Roles ─────────────────────────────────────────────────────────────────────
CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT UNIQUE NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Permissions ───────────────────────────────────────────────────────────────
CREATE TABLE permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT UNIQUE NOT NULL,   -- e.g. "products.write"
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Role ↔ Permission (many-to-many) ─────────────────────────────────────────
CREATE TABLE role_permissions (
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- ── User ↔ Role (many-to-many) ───────────────────────────────────────────────
CREATE TABLE user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ── Seed: default roles ───────────────────────────────────────────────────────
INSERT INTO roles (id, name, description) VALUES
    ('11111111-0000-0000-0000-000000000001', 'admin',     'Full system access'),
    ('11111111-0000-0000-0000-000000000002', 'manager',   'Operations manager'),
    ('11111111-0000-0000-0000-000000000003', 'warehouse',  'Inventory and shipments'),
    ('11111111-0000-0000-0000-000000000004', 'cashier',   'POS and order creation'),
    ('11111111-0000-0000-0000-000000000005', 'finance',   'Invoices and analytics');

-- ── Seed: permissions ─────────────────────────────────────────────────────────
INSERT INTO permissions (id, name, description) VALUES
    ('22222222-0000-0000-0000-000000000001', 'products.read',      'View products and variants'),
    ('22222222-0000-0000-0000-000000000002', 'products.write',     'Create and update products'),
    ('22222222-0000-0000-0000-000000000003', 'orders.manage',      'Create and process orders'),
    ('22222222-0000-0000-0000-000000000004', 'inventory.manage',   'Manage stock and batches'),
    ('22222222-0000-0000-0000-000000000005', 'returns.approve',    'Approve or reject returns'),
    ('22222222-0000-0000-0000-000000000006', 'analytics.view',     'View analytics and forecasts'),
    ('22222222-0000-0000-0000-000000000007', 'pricing.manage',     'Set and update channel prices'),
    ('22222222-0000-0000-0000-000000000008', 'suppliers.manage',   'Manage suppliers and POs'),
    ('22222222-0000-0000-0000-000000000009', 'users.manage',       'Create and manage users'),
    ('22222222-0000-0000-0000-000000000010', 'channels.manage',    'Connect and sync sales channels'),
    ('22222222-0000-0000-0000-000000000011', 'invoices.sandbox',   'Submit invoices to ASP sandbox');

-- ── Seed: role → permission assignments ──────────────────────────────────────
-- admin gets every permission
INSERT INTO role_permissions (role_id, permission_id)
SELECT '11111111-0000-0000-0000-000000000001', id FROM permissions;

-- manager
INSERT INTO role_permissions (role_id, permission_id)
SELECT '11111111-0000-0000-0000-000000000002', id FROM permissions
 WHERE name IN ('products.read','products.write','orders.manage','inventory.manage',
                'returns.approve','analytics.view','pricing.manage','suppliers.manage');

-- warehouse
INSERT INTO role_permissions (role_id, permission_id)
SELECT '11111111-0000-0000-0000-000000000003', id FROM permissions
 WHERE name IN ('products.read','inventory.manage','returns.approve','suppliers.manage');

-- cashier
INSERT INTO role_permissions (role_id, permission_id)
SELECT '11111111-0000-0000-0000-000000000004', id FROM permissions
 WHERE name IN ('products.read','orders.manage');

-- finance
INSERT INTO role_permissions (role_id, permission_id)
SELECT '11111111-0000-0000-0000-000000000005', id FROM permissions
 WHERE name IN ('analytics.view','invoices.sandbox','orders.manage');
