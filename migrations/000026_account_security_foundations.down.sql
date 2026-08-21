-- Refuse a rollback that would silently discard an active restriction which
-- cannot be represented by the legacy users columns.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM authentication_failure_states
    WHERE locked_until > now() AND mechanism <> 'password'
  ) THEN
    RAISE EXCEPTION 'cannot roll back: active non-password authentication restrictions exist';
  END IF;
END $$;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nodus_app') THEN
    GRANT UPDATE, DELETE ON audit_logs TO nodus_app;
  END IF;
END $$;

ALTER TABLE audit_logs
    DROP COLUMN privileged_reference,
    DROP COLUMN reason_code,
    DROP COLUMN user_agent,
    DROP COLUMN request_id,
    DROP COLUMN target_user_id;

DROP TABLE authentication_failure_states;
DROP TYPE authentication_mechanism;
