-- =============================================================================
-- Migration 008: Permission Version for Immediate Token Invalidation
--
-- Problem: JWT access tokens embed permissions at issuance time and remain
-- valid until expiry (15 min). If a role is added or removed, the existing
-- token retains stale permissions for up to 15 minutes.
--
-- Solution: Add a monotonically-incrementing permissions_version column to
-- the users table. This value is embedded in every JWT claim and verified
-- against the DB (via Redis cache) on each authenticated request. Any RBAC
-- mutation increments the version, immediately invalidating outstanding tokens.
-- =============================================================================

ALTER TABLE users
    ADD COLUMN permissions_version INT NOT NULL DEFAULT 1;

-- Set version = 1 for all existing users (already set by DEFAULT, but explicit
-- UPDATE makes the intent clear in case of partial migration scenarios).
UPDATE users SET permissions_version = 1 WHERE permissions_version IS NULL;

COMMENT ON COLUMN users.permissions_version IS
    'Incremented whenever roles or permissions for this user change. '
    'Embedded in JWT claims and verified on every request to ensure '
    'revoked permissions take effect immediately.';
