-- A login email identifies exactly one organization. lower(email) also prevents
-- case-only duplicates created by direct SQL or older application versions.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_tenant_email_key;
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));
