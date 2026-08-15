DROP FUNCTION IF EXISTS discover_active_organizations(TEXT);
DROP TABLE IF EXISTS organization_discovery_tokens;
DROP TABLE IF EXISTS organization_discovery_requests;
ALTER TABLE organizations DROP CONSTRAINT IF EXISTS organizations_slug_not_reserved;
