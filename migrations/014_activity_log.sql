-- =============================================================================
-- Migration 014: Activity Log
--
-- Audit trail for admin actions: product updates, return approvals, warehouse
-- transfers, etc. Tenant-scoped, immutable append-only log.
-- =============================================================================

CREATE TABLE activity_log (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID         NOT NULL REFERENCES tenants(id),
    actor_id     UUID         NOT NULL,
    actor_email  TEXT         NOT NULL,
    event_type   TEXT         NOT NULL,
    title        TEXT         NOT NULL,
    description  TEXT         NOT NULL DEFAULT '',
    subject_id   TEXT         NOT NULL DEFAULT '',
    subject_type TEXT         NOT NULL DEFAULT '',
    metadata     JSONB        DEFAULT '{}',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

CREATE INDEX idx_activity_log_tenant_created ON activity_log (tenant_id, created_at DESC);
CREATE INDEX idx_activity_log_subject ON activity_log (tenant_id, subject_type, subject_id);
CREATE INDEX idx_activity_log_event_type ON activity_log (tenant_id, event_type);

COMMENT ON TABLE activity_log IS 'Immutable audit log of admin actions. Never UPDATE or DELETE.';
