-- name: ListUsers :many
SELECT u.*,
    COALESCE(array_agg(DISTINCT r.name) FILTER (WHERE r.name IS NOT NULL), '{}')::text[] AS role_names,
    COALESCE(array_agg(DISTINCT p.code) FILTER (WHERE p.code IS NOT NULL), '{}')::text[] AS permission_codes,
    EXISTS (
        SELECT 1 FROM mfa_factors mf WHERE mf.user_id = u.id AND mf.confirmed_at IS NOT NULL
    ) AS mfa_enrolled,
    (SELECT i.expires_at FROM invitations i
     WHERE i.user_id = u.id AND i.tenant_id = u.tenant_id
     ORDER BY i.created_at DESC LIMIT 1) AS invitation_expires_at,
    (SELECT i.used_at FROM invitations i
     WHERE i.user_id = u.id AND i.tenant_id = u.tenant_id
     ORDER BY i.created_at DESC LIMIT 1) AS invitation_used_at
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
LEFT JOIN role_permissions rp ON rp.role_id = r.id
LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE u.tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
  AND (sqlc.narg(role)::text IS NULL OR EXISTS (
        SELECT 1 FROM user_roles ur2 JOIN roles r2 ON r2.id = ur2.role_id
        WHERE ur2.user_id = u.id
          AND ur2.tenant_id = u.tenant_id
          AND r2.tenant_id = u.tenant_id
          AND r2.name = sqlc.narg(role)
      ))
  AND (sqlc.narg(status)::user_status IS NULL OR u.status = sqlc.narg(status))
  AND (sqlc.narg(stale_access)::boolean IS NULL OR
       (sqlc.narg(stale_access)::boolean AND (u.next_access_review_due IS NULL OR u.next_access_review_due < now())) OR
       (NOT sqlc.narg(stale_access)::boolean AND NOT (u.next_access_review_due IS NULL OR u.next_access_review_due < now())))
GROUP BY u.id
ORDER BY u.full_name;

-- name: GetUserWithRolesByID :one
SELECT u.*,
    COALESCE(array_agg(DISTINCT r.name) FILTER (WHERE r.name IS NOT NULL), '{}')::text[] AS role_names,
    COALESCE(array_agg(DISTINCT p.code) FILTER (WHERE p.code IS NOT NULL), '{}')::text[] AS permission_codes,
    EXISTS (
        SELECT 1 FROM mfa_factors mf WHERE mf.user_id = u.id AND mf.confirmed_at IS NOT NULL
    ) AS mfa_enrolled,
    (SELECT i.expires_at FROM invitations i
     WHERE i.user_id = u.id AND i.tenant_id = u.tenant_id
     ORDER BY i.created_at DESC LIMIT 1) AS invitation_expires_at,
    (SELECT i.used_at FROM invitations i
     WHERE i.user_id = u.id AND i.tenant_id = u.tenant_id
     ORDER BY i.created_at DESC LIMIT 1) AS invitation_used_at
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
LEFT JOIN role_permissions rp ON rp.role_id = r.id
LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE u.id = $1
  AND u.tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
GROUP BY u.id;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: DeleteUserRoles :exec
DELETE FROM user_roles
WHERE user_id = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: InsertUserRole :exec
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u
JOIN roles r ON r.id = sqlc.arg(role_id)
WHERE u.id = sqlc.arg(user_id)
  AND u.tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
  AND r.tenant_id = u.tenant_id
ON CONFLICT DO NOTHING;

-- name: SetProviderIdentifier :exec
UPDATE users SET provider_identifier = $2
WHERE id = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: RecordAccessReview :exec
UPDATE users SET last_access_review_at = $2, next_access_review_due = $3
WHERE id = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: UnlockUser :exec
UPDATE users SET locked_until = NULL, failed_login_attempts = 0
WHERE id = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: GetRolesByIDs :many
SELECT * FROM roles
WHERE id::text = ANY(sqlc.arg(ids)::text[])
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: HasSuperuserRole :one
SELECT EXISTS (
    SELECT 1 FROM user_roles ur
    JOIN roles r ON r.id = ur.role_id
    WHERE ur.user_id = $1
      AND ur.tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
      AND r.tenant_id = ur.tenant_id
      AND r.is_superuser_role = true
);

-- name: CountOtherActiveSuperusers :one
SELECT count(*) FROM users u
WHERE u.id <> $1 AND u.status = 'active'
  AND u.tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
  AND EXISTS (
    SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
    WHERE ur.user_id = u.id AND ur.tenant_id = u.tenant_id
      AND r.tenant_id = u.tenant_id AND r.is_superuser_role = true
  );

-- name: LockUserLifecycle :exec
SELECT pg_advisory_xact_lock(hashtextextended(current_setting('app.tenant_id', true), 0));

-- name: DeactivateUser :exec
UPDATE users SET status = 'deactivated', deactivated_at = $2, deactivated_by = $3,
    deactivation_reason = $4, suspended_at = NULL, suspended_by = NULL, suspension_reason = NULL
WHERE id = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: SuspendUser :exec
UPDATE users SET status = 'suspended', suspended_at = $2, suspended_by = $3,
    suspension_reason = $4
WHERE id = $1 AND status <> 'suspended'
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: RestoreUser :exec
UPDATE users SET status = 'active', suspended_at = NULL, suspended_by = NULL,
    suspension_reason = NULL
WHERE id = $1 AND status = 'suspended'
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: RevokeSessionsByUser :exec
UPDATE sessions SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: RevokeRefreshTokensByUser :exec
UPDATE refresh_tokens SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: ConsumeLoginChallengesByUser :exec
UPDATE login_challenges SET consumed_at = now()
WHERE user_id = $1 AND consumed_at IS NULL
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: ConsumePasswordResetTokensByUser :exec
UPDATE password_reset_tokens SET used_at = now()
WHERE user_id = $1 AND used_at IS NULL
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: ConsumeEnrollmentTokensByUser :exec
UPDATE enrollment_tokens SET consumed_at = now()
WHERE user_id = $1 AND consumed_at IS NULL
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: ConsumeRecoverySessionsByUser :exec
UPDATE recovery_sessions SET consumed_at = now()
WHERE user_id = $1 AND consumed_at IS NULL
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid;

-- name: GetTemporaryRestrictionsByUser :many
SELECT mechanism::text AS mechanism, failure_count, next_attempt_at, locked_until
FROM authentication_failure_states
WHERE user_id = $1
  AND tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
  AND (locked_until > now() OR next_attempt_at > now())
ORDER BY mechanism;
