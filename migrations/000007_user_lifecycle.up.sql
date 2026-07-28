ALTER TYPE user_status ADD VALUE IF NOT EXISTS 'deactivated';

ALTER TABLE users ADD COLUMN deactivated_at TIMESTAMPTZ;

INSERT INTO permissions (id, code, description) VALUES
  ('00000000-0000-4000-8000-000000000013', 'users:deactivate', 'Deactivate and reactivate staff accounts')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description;

CREATE TABLE reactivation_tokens (
    id            UUID PRIMARY KEY,
    tenant_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    requested_by  UUID NOT NULL REFERENCES users(id),
    token_hash    TEXT NOT NULL UNIQUE,
    expires_at    TIMESTAMPTZ NOT NULL,
    used_at       TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_reactivation_tokens_user_id ON reactivation_tokens (user_id);

ALTER TABLE reactivation_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE reactivation_tokens FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON reactivation_tokens
  USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON reactivation_tokens
  FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id();
