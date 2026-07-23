-- name: GetRolesByIDs :many
SELECT * FROM roles WHERE id::text = ANY(sqlc.arg(ids)::text[]);

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: CreateInvitedUser :one
INSERT INTO users (id, full_name, username, email, provider_identifier, status)
VALUES ($1, $2, $3, $4, $5, 'invited')
RETURNING *;

-- name: AssignUserRole :exec
INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: GetUserRoleNames :many
SELECT r.name FROM roles r
JOIN user_roles ur ON ur.role_id = r.id
WHERE ur.user_id = $1
ORDER BY r.name;

-- name: CreateInvitation :exec
INSERT INTO invitations (id, user_id, invited_by, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetInvitationByTokenHash :one
SELECT * FROM invitations WHERE token_hash = $1;

-- name: GetLatestInvitationByUserID :one
SELECT * FROM invitations WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1;

-- name: ConsumeInvitation :exec
UPDATE invitations SET used_at = now() WHERE id = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: ActivateUserWithPassword :exec
UPDATE users SET status = 'active', password_hash = $2, password_changed_at = now() WHERE id = $1;

-- name: CreateEnrollmentToken :exec
INSERT INTO enrollment_tokens (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4);
