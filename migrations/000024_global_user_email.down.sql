DROP INDEX IF EXISTS users_email_lower_key;
ALTER TABLE users ADD CONSTRAINT users_tenant_email_key UNIQUE (tenant_id, email);
