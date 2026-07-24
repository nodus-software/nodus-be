DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['users','roles','mfa_factors','mfa_backup_codes',
    'login_challenges','sessions','refresh_tokens','password_reset_tokens',
    'password_reset_attempts','audit_logs','invitations','enrollment_tokens',
    'user_roles','role_permissions','organization_activation_tokens']
  LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS stamp_tenant_id ON %I', t);
    EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
    EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
  END LOOP;
END $$;

DROP FUNCTION IF EXISTS stamp_tenant_id();
DROP TABLE IF EXISTS organization_activation_tokens;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_tenant_username_key;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_tenant_email_key;
ALTER TABLE roles DROP CONSTRAINT IF EXISTS roles_tenant_name_key;

-- A downgrade can only restore v1's global uniqueness if upgraded data does
-- not contain duplicate values across tenants.
ALTER TABLE users ADD CONSTRAINT users_username_key UNIQUE (username);
ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);
ALTER TABLE roles ADD CONSTRAINT roles_name_key UNIQUE (name);

ALTER TABLE role_permissions DROP COLUMN tenant_id;
ALTER TABLE user_roles DROP COLUMN tenant_id;
ALTER TABLE enrollment_tokens DROP COLUMN tenant_id;
ALTER TABLE invitations DROP COLUMN tenant_id;
ALTER TABLE audit_logs DROP COLUMN tenant_id;
ALTER TABLE password_reset_attempts DROP COLUMN tenant_id;
ALTER TABLE password_reset_tokens DROP COLUMN tenant_id;
ALTER TABLE refresh_tokens DROP COLUMN tenant_id;
ALTER TABLE sessions DROP COLUMN tenant_id;
ALTER TABLE login_challenges DROP COLUMN tenant_id;
ALTER TABLE mfa_backup_codes DROP COLUMN tenant_id;
ALTER TABLE mfa_factors DROP COLUMN tenant_id;
ALTER TABLE roles DROP COLUMN tenant_id;
ALTER TABLE users DROP COLUMN tenant_id;

DROP TABLE organizations;
DROP TYPE organization_status;
