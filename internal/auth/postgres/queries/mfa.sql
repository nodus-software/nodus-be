-- name: CreateMFAFactor :one
INSERT INTO mfa_factors (id, user_id, type, label, secret_encrypted, public_key, confirmed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetMFAFactorByID :one
SELECT * FROM mfa_factors WHERE id = $1;

-- name: ListMFAFactorsByUser :many
SELECT * FROM mfa_factors WHERE user_id = $1 ORDER BY created_at;

-- name: ConfirmMFAFactor :exec
UPDATE mfa_factors SET confirmed_at = now() WHERE id = $1;

-- name: DeleteMFAFactor :exec
DELETE FROM mfa_factors WHERE id = $1;

-- name: CountConfirmedMFAFactors :one
SELECT count(*) FROM mfa_factors WHERE user_id = $1 AND confirmed_at IS NOT NULL;

-- name: CreateMFABackupCode :exec
INSERT INTO mfa_backup_codes (id, user_id, code_hash)
VALUES ($1, $2, $3);

-- name: GetUnusedMFABackupCodeByHash :one
SELECT * FROM mfa_backup_codes WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL;

-- name: ConsumeMFABackupCode :exec
UPDATE mfa_backup_codes SET used_at = now() WHERE id = $1 AND used_at IS NULL;

-- name: CountUnusedMFABackupCodes :one
SELECT count(*) FROM mfa_backup_codes WHERE user_id=$1 AND used_at IS NULL;

-- name: InvalidateMFABackupCodes :exec
UPDATE mfa_backup_codes SET used_at=COALESCE(used_at, now()) WHERE user_id=$1;

-- name: GetEnrollmentTokenByHash :one
SELECT id::text, user_id::text, expires_at,
       CASE WHEN consumed_at IS NULL THEN false ELSE true END::boolean AS consumed
FROM enrollment_tokens
WHERE token_hash = $1;

-- name: ConsumeEnrollmentToken :exec
UPDATE enrollment_tokens SET consumed_at = now()
WHERE id = $1 AND consumed_at IS NULL;

-- name: CreateWebAuthnCredential :exec
INSERT INTO webauthn_credentials(id,user_id,factor_id,credential_id,credential)
VALUES($1,$2,$3,$4,sqlc.arg(credential)::text::jsonb);

-- name: ListWebAuthnCredentialsByUser :many
SELECT * FROM webauthn_credentials WHERE user_id=$1 ORDER BY created_at;

-- name: GetWebAuthnCredentialByCredentialID :one
SELECT * FROM webauthn_credentials WHERE credential_id=$1;

-- name: UpdateWebAuthnCredential :exec
UPDATE webauthn_credentials SET credential=sqlc.arg(credential)::text::jsonb,updated_at=now()
WHERE credential_id=$1 AND user_id=$2;

-- name: CreateWebAuthnCeremony :exec
INSERT INTO webauthn_ceremonies(id,user_id,login_challenge_id,enrollment_token_id,purpose,label,session_data,expires_at)
VALUES($1,$2,$3,$4,$5,$6,sqlc.arg(session_data)::text::jsonb,$7);

-- name: GetWebAuthnCeremonyByID :one
SELECT * FROM webauthn_ceremonies WHERE id=$1;

-- name: ConsumeWebAuthnCeremony :execrows
UPDATE webauthn_ceremonies SET consumed_at=now()
WHERE id=$1 AND consumed_at IS NULL AND expires_at>now();

-- name: DeletePendingTOTPFactors :exec
DELETE FROM mfa_factors WHERE user_id=$1 AND type='totp' AND confirmed_at IS NULL;
