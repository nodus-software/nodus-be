-- Preflight before rollout (run before applying this migration):
-- SELECT u.tenant_id, u.id, u.email
-- FROM users u JOIN mfa_factors b ON b.user_id=u.id AND b.type='biometric' AND b.confirmed_at IS NOT NULL
-- WHERE NOT EXISTS (SELECT 1 FROM mfa_factors t WHERE t.user_id=u.id AND t.type='totp' AND t.confirmed_at IS NOT NULL);

DELETE FROM mfa_factors WHERE type = 'biometric';
ALTER TABLE mfa_factors DROP CONSTRAINT mfa_factor_secret_or_key;
ALTER TABLE mfa_factors ALTER COLUMN type TYPE text USING type::text;
DROP TYPE mfa_factor_type;
CREATE TYPE mfa_factor_type AS ENUM ('totp', 'webauthn');
ALTER TABLE mfa_factors ALTER COLUMN type TYPE mfa_factor_type USING type::mfa_factor_type;
ALTER TABLE mfa_factors ADD CONSTRAINT mfa_factor_shape CHECK (
  (type='totp' AND secret_encrypted IS NOT NULL AND public_key IS NULL) OR
  (type='webauthn' AND secret_encrypted IS NULL AND public_key IS NULL)
);
-- Older releases could create more than one abandoned pending setup. Pending
-- secrets have never authenticated a user, so retain only the newest record
-- before enforcing the one-pending-setup invariant.
WITH ranked AS (
  SELECT id, row_number() OVER (PARTITION BY user_id ORDER BY created_at DESC, id DESC) AS position
  FROM mfa_factors WHERE type='totp' AND confirmed_at IS NULL
)
DELETE FROM mfa_factors f USING ranked r WHERE f.id=r.id AND r.position>1;
CREATE UNIQUE INDEX one_confirmed_totp_per_user ON mfa_factors(user_id) WHERE type='totp' AND confirmed_at IS NOT NULL;
CREATE UNIQUE INDEX one_pending_totp_per_user ON mfa_factors(user_id) WHERE type='totp' AND confirmed_at IS NULL;

CREATE TABLE webauthn_credentials (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  factor_id UUID NOT NULL UNIQUE REFERENCES mfa_factors(id) ON DELETE CASCADE,
  credential_id BYTEA NOT NULL UNIQUE,
  credential JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX webauthn_credentials_user_idx ON webauthn_credentials(user_id);

CREATE TYPE webauthn_ceremony_purpose AS ENUM ('registration', 'authentication');
CREATE TABLE webauthn_ceremonies (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  login_challenge_id UUID REFERENCES login_challenges(id) ON DELETE CASCADE,
  enrollment_token_id UUID REFERENCES enrollment_tokens(id) ON DELETE CASCADE,
  purpose webauthn_ceremony_purpose NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  session_data JSONB NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX webauthn_ceremonies_expiry_idx ON webauthn_ceremonies(expires_at) WHERE consumed_at IS NULL;
CREATE INDEX webauthn_ceremonies_user_idx ON webauthn_ceremonies(user_id, purpose);

CREATE TABLE mfa_reset_tokens (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  requested_by UUID NOT NULL REFERENCES users(id),
  token_hash TEXT NOT NULL UNIQUE,
  reason TEXT NOT NULL CHECK (char_length(btrim(reason)) BETWEEN 1 AND 500),
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX mfa_reset_tokens_user_idx ON mfa_reset_tokens(user_id);
CREATE INDEX mfa_reset_tokens_expiry_idx ON mfa_reset_tokens(expires_at) WHERE consumed_at IS NULL;

DO $$ DECLARE t text; BEGIN
  FOREACH t IN ARRAY ARRAY['webauthn_credentials','webauthn_ceremonies','mfa_reset_tokens'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)', t);
    EXECUTE format('CREATE TRIGGER stamp_tenant_id BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION stamp_tenant_id()', t);
  END LOOP;
END $$;

INSERT INTO permissions(id, code, description) VALUES
('00000000-0000-4000-8000-000000000014','users:mfa_reset','Approve another user''s MFA reset')
ON CONFLICT (code) DO NOTHING;
