-- name: GetRolesByIDs :many
SELECT * FROM roles
WHERE id::text = ANY(sqlc.arg(ids)::text[])
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE tenant_id = sqlc.arg(tenant_id)
  AND email = sqlc.arg(email);

-- name: CreateInvitedUser :one
INSERT INTO users (id, full_name, username, email, provider_identifier, status)
VALUES ($1, $2, $3, $4, $5, 'invited')
RETURNING *;

-- name: AssignUserRole :exec
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u
JOIN roles r ON r.id = sqlc.arg(role_id)
WHERE u.id = sqlc.arg(user_id)
  AND u.tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
  AND r.tenant_id = u.tenant_id
ON CONFLICT DO NOTHING;

-- name: GetUserRoleNames :many
SELECT r.name FROM roles r
JOIN user_roles ur ON ur.role_id = r.id
WHERE ur.user_id = $1
  AND ur.tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
  AND r.tenant_id = ur.tenant_id
ORDER BY r.name;

-- name: CreateInvitation :exec
INSERT INTO invitations (id, user_id, invited_by, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetInvitationByTokenHash :one
SELECT * FROM invitations
WHERE token_hash = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: GetLatestInvitationByUserID :one
SELECT * FROM invitations
WHERE user_id = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
ORDER BY created_at DESC LIMIT 1;

-- name: ConsumeInvitation :exec
UPDATE invitations SET used_at = now()
WHERE id = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: ActivateUserWithPassword :exec
UPDATE users SET status = 'active', password_hash = $2, password_changed_at = now()
WHERE id = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: RestoreInvitedUser :exec
UPDATE users SET status = 'invited'
WHERE id = $1
  AND password_hash IS NULL
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: CreateEnrollmentToken :exec
INSERT INTO enrollment_tokens (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4);

-- name: CreateReactivationToken :exec
INSERT INTO reactivation_tokens (id, user_id, requested_by, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetReactivationTokenByHash :one
SELECT * FROM reactivation_tokens
WHERE token_hash = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: ConsumeReactivationToken :execrows
UPDATE reactivation_tokens SET used_at = now()
WHERE id = $1 AND used_at IS NULL AND expires_at > now()
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: ConsumeReactivationTokensByUser :exec
UPDATE reactivation_tokens SET used_at = now()
WHERE user_id = $1 AND used_at IS NULL
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: ResetMFAByUser :exec
DELETE FROM mfa_factors
WHERE user_id = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: ResetMFABackupCodesByUser :exec
DELETE FROM mfa_backup_codes
WHERE user_id = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: ActivateReactivatedUser :exec
UPDATE users
SET status = 'active', password_hash = $2, password_changed_at = now(),
    deactivated_at = NULL, deactivated_by = NULL, deactivation_reason = NULL,
    last_access_review_at = $3, next_access_review_due = $4
WHERE id = $1 AND status = 'deactivated'
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: DeletePendingUser :exec
DELETE FROM users
WHERE id = $1 AND status = 'invited' AND password_hash IS NULL
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;
