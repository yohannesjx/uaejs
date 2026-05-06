-- =============================================================================
-- Migration 004: ASP Sandbox Status + Batch Import Jobs
-- =============================================================================

-- ── ASP sandbox result column on order_invoices ───────────────────────────
CREATE TYPE sandbox_status AS ENUM ('pending', 'accepted', 'rejected', 'error');

ALTER TABLE order_invoices
  ADD COLUMN IF NOT EXISTS sandbox_status    sandbox_status,
  ADD COLUMN IF NOT EXISTS sandbox_resp_id   TEXT,
  ADD COLUMN IF NOT EXISTS sandbox_errors    TEXT[],
  ADD COLUMN IF NOT EXISTS sandbox_submitted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_order_invoices_sandbox_status
  ON order_invoices(sandbox_status) WHERE sandbox_status IS NOT NULL;

-- ── Batch import jobs ─────────────────────────────────────────────────────
-- Tracks each CSV/JSON import request so imports are idempotent and auditable.
CREATE TYPE import_status AS ENUM ('pending', 'processing', 'completed', 'failed');

CREATE TABLE batch_imports (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    filename        TEXT NOT NULL,
    imported_by     TEXT NOT NULL,           -- admin username / API key fingerprint
    status          import_status NOT NULL DEFAULT 'pending',
    total_rows      INTEGER,
    imported_rows   INTEGER DEFAULT 0,
    failed_rows     INTEGER DEFAULT 0,
    error_details   JSONB,                   -- [{row, sku, error}] for failed rows
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_batch_imports_status ON batch_imports(status);

CREATE TRIGGER trg_batch_imports_updated_at
    BEFORE UPDATE ON batch_imports
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
