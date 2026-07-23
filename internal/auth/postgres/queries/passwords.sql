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
