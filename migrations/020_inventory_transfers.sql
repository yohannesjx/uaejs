-- =============================================================================
-- Migration 020: Inventory Transfers lifecycle
-- =============================================================================

DO $$ BEGIN
  CREATE TYPE transfer_status AS ENUM ('draft', 'pending', 'in_transit', 'completed', 'cancelled');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS inventory_transfers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  reference TEXT NOT NULL DEFAULT '',
  origin_warehouse_id UUID NOT NULL REFERENCES warehouses(id),
  destination_warehouse_id UUID NOT NULL REFERENCES warehouses(id),
  status transfer_status NOT NULL DEFAULT 'draft',
  notes TEXT,
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_transfer_warehouses_distinct CHECK (origin_warehouse_id <> destination_warehouse_id)
);

CREATE INDEX IF NOT EXISTS idx_inventory_transfers_tenant_created
  ON inventory_transfers (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_inventory_transfers_status
  ON inventory_transfers (tenant_id, status);

CREATE TABLE IF NOT EXISTS inventory_transfer_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  transfer_id UUID NOT NULL REFERENCES inventory_transfers(id) ON DELETE CASCADE,
  variant_id UUID NOT NULL REFERENCES variants(id),
  quantity INT NOT NULL CHECK (quantity > 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (transfer_id, variant_id)
);

CREATE INDEX IF NOT EXISTS idx_inventory_transfer_items_transfer
  ON inventory_transfer_items (transfer_id);
