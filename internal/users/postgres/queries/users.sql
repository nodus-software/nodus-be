-- name: ListUsers :many
SELECT u.*,
    COALESCE(array_agg(DISTINCT r.name) FILTER (WHERE r.name IS NOT NULL), '{}')::text[] AS role_names,
    COALESCE(array_agg(DISTINCT p.code) FILTER (WHERE p.code IS NOT NULL), '{}')::text[] AS permission_codes,
    EXISTS (
        SELECT 1 FROM mfa_factors mf WHERE mf.user_id = u.id AND mf.confirmed_at IS NOT NULL
    ) AS mfa_enrolled
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
LEFT JOIN role_permissions rp ON rp.role_id = r.id
LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE (sqlc.narg(role)::text IS NULL OR EXISTS (
        SELECT 1 FROM user_roles ur2 JOIN roles r2 ON r2.id = ur2.role_id
        WHERE ur2.user_id = u.id AND r2.name = sqlc.narg(role)
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
    ) AS mfa_enrolled
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
LEFT JOIN role_permissions rp ON rp.role_id = r.id
LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE u.id = $1
GROUP BY u.id;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: DeleteUserRoles :exec
DELETE FROM user_roles WHERE user_id = $1;

-- name: InsertUserRole :exec
INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: UpdateUserStatus :exec
UPDATE users SET status = $2 WHERE id = $1;

-- name: SetProviderIdentifier :exec
UPDATE users SET provider_identifier = $2 WHERE id = $1;

-- name: RecordAccessReview :exec
UPDATE users SET last_access_review_at = $2, next_access_review_due = $3 WHERE id = $1;

-- name: UnlockUser :exec
UPDATE users SET locked_until = NULL, failed_login_attempts = 0 WHERE id = $1;

-- name: GetRolesByIDs :many
SELECT * FROM roles WHERE id::text = ANY(sqlc.arg(ids)::text[]);

-- name: HasSuperuserRole :one
SELECT EXISTS (
    SELECT 1 FROM user_roles ur
    JOIN roles r ON r.id = ur.role_id
    WHERE ur.user_id = $1 AND r.is_superuser_role = true
);
