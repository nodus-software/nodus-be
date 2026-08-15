-- Reserve infrastructure hostnames and support privacy-preserving organization discovery.
ALTER TABLE organizations ADD CONSTRAINT organizations_slug_not_reserved CHECK (
  slug <> ALL (ARRAY['app','api','www','admin','mail','info','noreply','support','status'])
);

CREATE TABLE organization_discovery_requests (
    id          UUID PRIMARY KEY,
    email_hash  TEXT NOT NULL,
    ip_address  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_organization_discovery_email_rate ON organization_discovery_requests(email_hash, created_at);
CREATE INDEX idx_organization_discovery_ip_rate ON organization_discovery_requests(ip_address, created_at);

CREATE TABLE organization_discovery_tokens (
    id          UUID PRIMARY KEY,
    email       TEXT NOT NULL,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- This is deliberately the only cross-tenant user lookup available to the API.
-- It returns no user identifiers, credentials, roles, or clinical data.
CREATE FUNCTION discover_active_organizations(discovery_email TEXT)
RETURNS TABLE(id UUID, organization_name TEXT, slug TEXT)
LANGUAGE sql
SECURITY DEFINER
SET search_path = public, pg_temp
SET row_security = off
AS $$
  SELECT DISTINCT o.id, o.organization_name, o.slug
  FROM users u
  JOIN organizations o ON o.id = u.tenant_id
  WHERE lower(u.email) = lower(discovery_email)
    AND u.status = 'active'
    AND o.status = 'active'
  ORDER BY o.organization_name, o.slug
$$;

REVOKE ALL ON FUNCTION discover_active_organizations(TEXT) FROM PUBLIC;
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nodus_app') THEN
    GRANT EXECUTE ON FUNCTION discover_active_organizations(TEXT) TO nodus_app;
  END IF;
END $$;
