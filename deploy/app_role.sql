-- Runtime database role for the API.
--
-- Every tenant-scoped table relies on its `tenant_isolation` row-level security
-- policy for scoping — the clinical queries carry no tenant predicate of their
-- own. PostgreSQL exempts superusers and roles with BYPASSRLS from those
-- policies, so an API connected as the owning/superuser role sees *every*
-- tenant's rows and the isolation is silently inert.
--
-- This creates the least-privileged role docs/deployment.md expects at runtime
-- (`app_user` there, `nodus_app` locally). Migrations keep using the owning role
-- via MIGRATION_DB_URL; only DB_URL points here.
--
-- Idempotent: safe to re-run after new migrations add tables.

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nodus_app') THEN
    CREATE ROLE nodus_app LOGIN PASSWORD 'nodus';
  END IF;
END $$;

-- Belt and braces: neither may ever be granted, or RLS stops applying.
ALTER ROLE nodus_app NOSUPERUSER NOBYPASSRLS;

GRANT USAGE ON SCHEMA public TO nodus_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO nodus_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO nodus_app;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO nodus_app;

-- Tables created by later migrations are granted automatically, so this file
-- does not have to be re-run every time the schema grows.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO nodus_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO nodus_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT EXECUTE ON FUNCTIONS TO nodus_app;

-- Audit records are application-level append-only. Apply this after the broad
-- table grants above each time the role script is refreshed.
REVOKE UPDATE, DELETE ON audit_logs FROM nodus_app;
