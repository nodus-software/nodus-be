CREATE TYPE authentication_mechanism AS ENUM ('password', 'mfa', 'recovery', 'captcha');

CREATE TABLE authentication_failure_states (
    tenant_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mechanism          authentication_mechanism NOT NULL,
    failure_count      INTEGER NOT NULL DEFAULT 0 CHECK (failure_count >= 0),
    window_started_at  TIMESTAMPTZ NOT NULL,
    last_failure_at    TIMESTAMPTZ NOT NULL,
    next_attempt_at    TIMESTAMPTZ,
    locked_until       TIMESTAMPTZ,
    lock_cycle_count   INTEGER NOT NULL DEFAULT 0 CHECK (lock_cycle_count >= 0),
    cycle_window_start TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, mechanism)
);

CREATE INDEX idx_auth_failure_states_restrictions
    ON authentication_failure_states (tenant_id, locked_until)
    WHERE locked_until IS NOT NULL;

CREATE TRIGGER authentication_failure_states_set_updated_at
    BEFORE UPDATE ON authentication_failure_states
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE authentication_failure_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE authentication_failure_states FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON authentication_failure_states
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON authentication_failure_states
    FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id();

-- Preserve active legacy restrictions for compatibility while the application
-- observes mechanism-specific decisions without enforcing them.
INSERT INTO authentication_failure_states (
    tenant_id, user_id, mechanism, failure_count, window_started_at,
    last_failure_at, locked_until, lock_cycle_count, cycle_window_start
)
SELECT tenant_id, id, 'password', GREATEST(failed_login_attempts, 1),
       now(), now(), locked_until, 1, now()
FROM users
WHERE locked_until > now();

ALTER TABLE audit_logs
    ADD COLUMN target_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN request_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN user_agent TEXT NOT NULL DEFAULT '',
    ADD COLUMN reason_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN privileged_reference TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_audit_logs_target_user_id ON audit_logs (target_user_id);
CREATE INDEX idx_audit_logs_request_id ON audit_logs (request_id) WHERE request_id <> '';

-- The runtime role may append and read audit records, but cannot mutate them.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nodus_app') THEN
    REVOKE UPDATE, DELETE ON audit_logs FROM nodus_app;
  END IF;
END $$;
