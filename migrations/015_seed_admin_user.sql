-- =============================================================================
-- Migration 015: Seed default admin user
--
-- Creates admin@admin.ae / password for initial login.
-- Safe to run multiple times (ON CONFLICT DO NOTHING).
-- =============================================================================

INSERT INTO users (id, email, password_hash, full_name, is_active, permissions_version, created_at, updated_at)
VALUES (
  '33333333-0000-0000-0000-000000000001',
  'admin@admin.ae',
  '$2a$10$kDIZGoytJf8gTsWrD3zTO.UTNwH1oXhhG7Ftv5w88gS1ydjqUF0Iy',
  'Admin',
  TRUE,
  1,
  NOW() AT TIME ZONE 'UTC',
  NOW() AT TIME ZONE 'UTC'
)
ON CONFLICT (email) DO NOTHING;

INSERT INTO user_roles (user_id, role_id)
SELECT '33333333-0000-0000-0000-000000000001', '11111111-0000-0000-0000-000000000001'
WHERE EXISTS (SELECT 1 FROM users WHERE id = '33333333-0000-0000-0000-000000000001')
  AND NOT EXISTS (SELECT 1 FROM user_roles WHERE user_id = '33333333-0000-0000-0000-000000000001' AND role_id = '11111111-0000-0000-0000-000000000001');
