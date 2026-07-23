-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, device_label, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5);

-- name: GetSessionByID :one
SELECT * FROM sessions WHERE id = $1;

-- name: ListActiveSessionsByUser :many
SELECT * FROM sessions
WHERE user_id = $1 AND revoked_at IS NULL
ORDER BY last_active_at DESC;

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = now() WHERE id = $1;

-- name: RevokeSessionsByUser :exec
UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL;

-- name: RevokeSessionsByUserExceptSession :exec
UPDATE sessions
SET revoked_at = now()
WHERE user_id = $1 AND id != $2 AND revoked_at IS NULL;

-- name: TouchSessionLastActive :exec
UPDATE sessions SET last_active_at = now() WHERE id = $1;
