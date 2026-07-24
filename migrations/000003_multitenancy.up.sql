CREATE TYPE organization_status AS ENUM ('pending', 'active', 'suspended');

CREATE TABLE organizations (
    id                 UUID PRIMARY KEY,
    organization_name  TEXT NOT NULL,
    slug               TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9-]{3,32}$'),
    status             organization_status NOT NULL DEFAULT 'pending',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER organizations_set_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Preserve installations upgraded from v1 by placing existing records in a
-- quarantined legacy tenant. Operators can rename/activate it deliberately.
INSERT INTO organizations (id, organization_name, slug, status)
VALUES ('00000000-0000-0000-0000-000000000001', 'Legacy organization', 'legacy', 'suspended');

ALTER TABLE users ADD COLUMN tenant_id UUID REFERENCES organizations(id);
ALTER TABLE roles ADD COLUMN tenant_id UUID REFERENCES organizations(id);
ALTER TABLE mfa_factors ADD COLUMN tenant_id UUID REFERENCES organizations(id);
ALTER TABLE mfa_backup_codes ADD COLUMN tenant_id UUID REFERENCES organizations(id);
ALTER TABLE login_challenges ADD COLUMN tenant_id UUID REFERENCES organizations(id);
ALTER TABLE sessions ADD COLUMN tenant_id UUID REFERENCES organizations(id);
ALTER TABLE refresh_tokens ADD COLUMN tenant_id UUID REFERENCES organizations(id);
ALTER TABLE password_reset_tokens ADD COLUMN tenant_id UUID REFERENCES organizations(id);
ALTER TABLE password_reset_attempts ADD COLUMN tenant_id UUID REFERENCES organizations(id);
ALTER TABLE audit_logs ADD COLUMN tenant_id UUID REFERENCES organizations(id);
ALTER TABLE invitations ADD COLUMN tenant_id UUID REFERENCES organizations(id);
ALTER TABLE enrollment_tokens ADD COLUMN tenant_id UUID REFERENCES organizations(id);
ALTER TABLE user_roles ADD COLUMN tenant_id UUID REFERENCES organizations(id);
ALTER TABLE role_permissions ADD COLUMN tenant_id UUID REFERENCES organizations(id);

UPDATE users SET tenant_id = '00000000-0000-0000-0000-000000000001';
UPDATE roles SET tenant_id = '00000000-0000-0000-0000-000000000001';
UPDATE mfa_factors f SET tenant_id = u.tenant_id FROM users u WHERE u.id = f.user_id;
UPDATE mfa_backup_codes c SET tenant_id = u.tenant_id FROM users u WHERE u.id = c.user_id;
UPDATE login_challenges c SET tenant_id = u.tenant_id FROM users u WHERE u.id = c.user_id;
UPDATE sessions s SET tenant_id = u.tenant_id FROM users u WHERE u.id = s.user_id;
UPDATE refresh_tokens t SET tenant_id = u.tenant_id FROM users u WHERE u.id = t.user_id;
UPDATE password_reset_tokens t SET tenant_id = u.tenant_id FROM users u WHERE u.id = t.user_id;
UPDATE password_reset_attempts SET tenant_id = '00000000-0000-0000-0000-000000000001';
UPDATE audit_logs SET tenant_id = '00000000-0000-0000-0000-000000000001';
UPDATE invitations i SET tenant_id = u.tenant_id FROM users u WHERE u.id = i.user_id;
UPDATE enrollment_tokens t SET tenant_id = u.tenant_id FROM users u WHERE u.id = t.user_id;
UPDATE user_roles ur SET tenant_id = u.tenant_id FROM users u WHERE u.id = ur.user_id;
UPDATE role_permissions rp SET tenant_id = r.tenant_id FROM roles r WHERE r.id = rp.role_id;

ALTER TABLE users ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE roles ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE mfa_factors ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE mfa_backup_codes ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE login_challenges ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE sessions ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE refresh_tokens ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE password_reset_tokens ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE password_reset_attempts ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE audit_logs ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE invitations ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE enrollment_tokens ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE user_roles ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE role_permissions ALTER COLUMN tenant_id SET NOT NULL;

ALTER TABLE users DROP CONSTRAINT users_username_key;
ALTER TABLE users DROP CONSTRAINT users_email_key;
ALTER TABLE roles DROP CONSTRAINT roles_name_key;
ALTER TABLE users ADD CONSTRAINT users_tenant_username_key UNIQUE (tenant_id, username);
ALTER TABLE users ADD CONSTRAINT users_tenant_email_key UNIQUE (tenant_id, email);
ALTER TABLE roles ADD CONSTRAINT roles_tenant_name_key UNIQUE (tenant_id, name);
CREATE INDEX idx_users_tenant_id ON users (tenant_id);
CREATE INDEX idx_roles_tenant_id ON roles (tenant_id);
CREATE INDEX idx_audit_logs_tenant_id ON audit_logs (tenant_id);

CREATE TABLE organization_activation_tokens (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- RLS is defense in depth. Application queries also carry tenant_id. The
-- transaction-local setting is installed by tenant-aware repository calls.
DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['users','roles','mfa_factors','mfa_backup_codes',
    'login_challenges','sessions','refresh_tokens','password_reset_tokens',
    'password_reset_attempts','audit_logs','invitations','enrollment_tokens',
    'user_roles','role_permissions','organization_activation_tokens']
  LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format(
      'CREATE POLICY tenant_isolation ON %I USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)',
      t);
  END LOOP;
END $$;

CREATE OR REPLACE FUNCTION stamp_tenant_id() RETURNS TRIGGER AS $$
BEGIN
  IF NEW.tenant_id IS NULL THEN
    NEW.tenant_id := NULLIF(current_setting('app.tenant_id', true), '')::uuid;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['users','roles','mfa_factors','mfa_backup_codes',
    'login_challenges','sessions','refresh_tokens','password_reset_tokens',
    'password_reset_attempts','audit_logs','invitations','enrollment_tokens',
    'user_roles','role_permissions','organization_activation_tokens']
  LOOP
    EXECUTE format('CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id()', t);
  END LOOP;
END $$;
