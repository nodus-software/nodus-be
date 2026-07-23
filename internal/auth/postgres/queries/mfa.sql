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
UPDATE mfa_backup_codes SET used_at = now() WHERE id = $1;
