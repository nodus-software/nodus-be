DROP TABLE IF EXISTS reactivation_tokens;
DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE code = 'users:deactivate');
DELETE FROM permissions WHERE code = 'users:deactivate';
ALTER TABLE users DROP COLUMN IF EXISTS deactivated_at;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM users WHERE status::text = 'deactivated') THEN
    RAISE EXCEPTION 'cannot roll back while deactivated users exist';
  END IF;
END $$;
ALTER TYPE user_status RENAME TO user_status_with_deactivated;
CREATE TYPE user_status AS ENUM ('invited', 'active', 'suspended', 'pending_review');
ALTER TABLE users ALTER COLUMN status DROP DEFAULT;
ALTER TABLE users ALTER COLUMN status TYPE user_status USING status::text::user_status;
ALTER TABLE users ALTER COLUMN status SET DEFAULT 'invited';
DROP TYPE user_status_with_deactivated;
