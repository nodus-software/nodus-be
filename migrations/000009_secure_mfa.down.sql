-- Deleted legacy biometric keys cannot be recreated by rollback.
DELETE FROM role_permissions WHERE permission_id=(SELECT id FROM permissions WHERE code='users:mfa_reset');
DELETE FROM permissions WHERE code='users:mfa_reset';
DROP TABLE IF EXISTS mfa_reset_tokens;
DROP TABLE IF EXISTS webauthn_ceremonies;
DROP TYPE IF EXISTS webauthn_ceremony_purpose;
DROP TABLE IF EXISTS webauthn_credentials;
DROP INDEX IF EXISTS one_pending_totp_per_user;
DROP INDEX IF EXISTS one_confirmed_totp_per_user;
ALTER TABLE mfa_factors DROP CONSTRAINT mfa_factor_shape;
ALTER TABLE mfa_factors ALTER COLUMN type TYPE text USING type::text;
DROP TYPE mfa_factor_type;
CREATE TYPE mfa_factor_type AS ENUM ('totp','biometric');
DELETE FROM mfa_factors WHERE type='webauthn';
ALTER TABLE mfa_factors ALTER COLUMN type TYPE mfa_factor_type USING type::mfa_factor_type;
ALTER TABLE mfa_factors ADD CONSTRAINT mfa_factor_secret_or_key CHECK ((type='totp' AND secret_encrypted IS NOT NULL) OR (type='biometric' AND public_key IS NOT NULL));
