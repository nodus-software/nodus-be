-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: IncrementFailedLoginAttempts :one
UPDATE users
SET failed_login_attempts = failed_login_attempts + 1
WHERE id = $1
RETURNING failed_login_attempts;

-- name: LockUser :exec
UPDATE users
SET locked_until = $2
WHERE id = $1;

-- name: UnlockUser :exec
UPDATE users
SET locked_until = NULL, failed_login_attempts = 0
WHERE id = $1;

-- name: ResetFailedLoginAttempts :exec
UPDATE users
SET failed_login_attempts = 0
WHERE id = $1;

-- name: CreateLoginChallenge :exec
INSERT INTO login_challenges (id, user_id, challenge_token_hash, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetLoginChallengeByHash :one
SELECT * FROM login_challenges WHERE challenge_token_hash = $1;

-- name: ConsumeLoginChallenge :exec
UPDATE login_challenges
SET consumed_at = now()
WHERE id = $1;
