-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (id, session_id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetRefreshTokenByHash :one
SELECT * FROM refresh_tokens WHERE token_hash = $1;

-- name: RevokeRefreshToken :execrows
UPDATE refresh_tokens SET revoked_at = now()
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeRefreshTokensByUser :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL;

-- name: RevokeRefreshTokensByUserExceptSession :exec
UPDATE refresh_tokens
SET revoked_at = now()
WHERE user_id = $1 AND session_id != $2 AND revoked_at IS NULL;

-- name: RevokeRefreshTokensBySession :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE session_id = $1 AND revoked_at IS NULL;
