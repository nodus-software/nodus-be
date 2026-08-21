DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM recovery_sessions WHERE consumed_at IS NULL AND expires_at > now())
     OR EXISTS (SELECT 1 FROM recovery_email_tokens WHERE consumed_at IS NULL AND expires_at > now()) THEN
    RAISE EXCEPTION 'cannot roll back: active account recovery state exists';
  END IF;
END $$;

DROP TABLE recovery_sessions;
DROP TABLE recovery_email_tokens;
DROP TYPE recovery_intent;
