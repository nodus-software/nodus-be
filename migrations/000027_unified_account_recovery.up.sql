CREATE TYPE recovery_intent AS ENUM ('password', 'mfa', 'both');

CREATE TABLE recovery_email_tokens (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    intent recovery_intent NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX one_active_recovery_email_token ON recovery_email_tokens (tenant_id, user_id, intent) WHERE consumed_at IS NULL;

CREATE TABLE recovery_sessions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    can_reset_password BOOLEAN NOT NULL,
    can_replace_mfa BOOLEAN NOT NULL,
    password_completed_at TIMESTAMPTZ,
    mfa_completed_at TIMESTAMPTZ,
    failed_attempts INTEGER NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE recovery_email_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE recovery_email_tokens FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON recovery_email_tokens USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON recovery_email_tokens FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id();
ALTER TABLE recovery_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE recovery_sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON recovery_sessions USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON recovery_sessions FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id();
