-- name: UpdatePasswordHash :exec
UPDATE users SET password_hash = $2, password_changed_at = now() WHERE id = $1;

-- name: CreatePasswordResetToken :exec
INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetPasswordResetTokenByHash :one
SELECT * FROM password_reset_tokens WHERE token_hash = $1;

-- name: ConsumePasswordResetToken :exec
UPDATE password_reset_tokens SET used_at = now() WHERE id = $1;

-- name: InvalidateOtherPasswordResetTokens :exec
UPDATE password_reset_tokens
SET used_at = now()
WHERE user_id = $1 AND id != $2 AND used_at IS NULL;

-- name: RecordPasswordResetAttempt :exec
INSERT INTO password_reset_attempts (id, username_attempted, ip_address)
VALUES ($1, $2, $3);

-- name: CountPasswordResetAttemptsByUsername :one
SELECT count(*) FROM password_reset_attempts
WHERE username_attempted = $1 AND created_at > $2;

-- name: CountPasswordResetAttemptsByIP :one
SELECT count(*) FROM password_reset_attempts
WHERE ip_address = $1 AND created_at > $2;

-- name: InvalidateRecoveryEmailTokens :exec
UPDATE recovery_email_tokens SET consumed_at = now() WHERE user_id = $1 AND intent = $2 AND consumed_at IS NULL;

-- name: CreateRecoveryEmailToken :exec
INSERT INTO recovery_email_tokens (id, user_id, intent, token_hash, expires_at) VALUES ($1, $2, $3, $4, $5);

-- name: ConsumeRecoveryEmailToken :one
UPDATE recovery_email_tokens SET consumed_at = now()
WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > now() RETURNING *;

-- name: CreateRecoverySession :exec
INSERT INTO recovery_sessions (id, user_id, token_hash, can_reset_password, can_replace_mfa, expires_at) VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetRecoverySessionByHash :one
SELECT * FROM recovery_sessions WHERE token_hash = $1;

-- name: IncrementRecoverySessionFailure :one
UPDATE recovery_sessions SET failed_attempts = failed_attempts + 1 WHERE id = $1 RETURNING failed_attempts;

-- name: CompleteRecoveryPassword :execrows
UPDATE recovery_sessions SET password_completed_at = now(), consumed_at = CASE WHEN NOT can_replace_mfa OR mfa_completed_at IS NOT NULL THEN now() ELSE consumed_at END
WHERE id = $1 AND password_completed_at IS NULL AND consumed_at IS NULL;

-- name: CompleteRecoveryMFA :execrows
UPDATE recovery_sessions SET mfa_completed_at = now(), consumed_at = CASE WHEN NOT can_reset_password OR password_completed_at IS NOT NULL THEN now() ELSE consumed_at END
WHERE id = $1 AND mfa_completed_at IS NULL AND consumed_at IS NULL;

-- name: InvalidateRecoverySessionsByUser :exec
UPDATE recovery_sessions SET consumed_at = now() WHERE user_id = $1 AND consumed_at IS NULL AND id != $2;
