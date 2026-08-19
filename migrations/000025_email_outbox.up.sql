CREATE TABLE email_outbox (
    id                  UUID PRIMARY KEY,
    tenant_id           UUID REFERENCES organizations(id) ON DELETE CASCADE,
    kind                TEXT NOT NULL,
    recipient           TEXT,
    recipient_hash      TEXT NOT NULL,
    subject             TEXT,
    text_body           TEXT,
    html_body           TEXT,
    status              TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'expired')),
    attempt_count       INTEGER NOT NULL DEFAULT 0,
    next_attempt_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_until         TIMESTAMPTZ,
    last_provider       TEXT,
    provider_message_id TEXT,
    last_error          TEXT,
    expires_at          TIMESTAMPTZ,
    sent_at             TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_email_outbox_ready
    ON email_outbox (next_attempt_at, created_at)
    WHERE status IN ('pending', 'processing');

CREATE INDEX idx_email_outbox_cleanup
    ON email_outbox (updated_at)
    WHERE status IN ('sent', 'failed', 'expired');

CREATE TRIGGER email_outbox_set_updated_at
    BEFORE UPDATE ON email_outbox
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE email_outbox IS
    'Global operational queue. It intentionally has no tenant RLS because workers claim mail across all tenants; no HTTP endpoint exposes it.';

-- This is the pending-registration counterpart to discover_active_organizations:
-- it exposes only the fields required to reissue the founding administrator's
-- activation link while the caller still has no tenant context.
CREATE FUNCTION discover_pending_registration(discovery_email TEXT)
RETURNS TABLE(tenant_id UUID, organization_name TEXT, slug TEXT, user_id UUID, full_name TEXT, email TEXT)
LANGUAGE sql
SECURITY DEFINER
SET search_path = public, pg_temp
SET row_security = off
AS $$
  SELECT o.id, o.organization_name, o.slug, u.id, u.full_name, u.email
  FROM users u
  JOIN organizations o ON o.id = u.tenant_id
  JOIN user_roles ur ON ur.user_id = u.id AND ur.tenant_id = u.tenant_id
  JOIN roles r ON r.id = ur.role_id AND r.tenant_id = u.tenant_id
  WHERE lower(u.email) = lower(discovery_email)
    AND o.status = 'pending'
    AND u.status = 'invited'
    AND u.password_hash IS NULL
    AND r.is_superuser_role = true
  LIMIT 1
$$;

REVOKE ALL ON FUNCTION discover_pending_registration(TEXT) FROM PUBLIC;
