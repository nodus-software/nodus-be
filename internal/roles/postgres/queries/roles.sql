-- name: ListRolesWithPermissions :many
SELECT r.id, r.tenant_id, r.name, r.description, r.is_superuser_role, r.requires_provider_identifier,
    COALESCE(array_agg(DISTINCT p.code) FILTER (WHERE p.code IS NOT NULL), '{}')::text[] AS permission_codes
FROM roles r
LEFT JOIN role_permissions rp ON rp.role_id = r.id
LEFT JOIN permissions p ON p.id = rp.permission_id
GROUP BY r.id
ORDER BY r.name;

-- name: GetRoleByID :one
SELECT * FROM roles WHERE id = $1;

-- name: GetRolesByIDs :many
SELECT * FROM roles WHERE id::text = ANY(sqlc.arg(ids)::text[]);

-- name: CreateRole :one
INSERT INTO roles (id, name, description, is_superuser_role, requires_provider_identifier)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPermissionsByCodes :many
SELECT * FROM permissions WHERE code = ANY(sqlc.arg(codes)::text[]);

-- name: AddRolePermission :exec
INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: HasSuperuserRole :one
SELECT EXISTS (
    SELECT 1 FROM user_roles ur
    JOIN roles r ON r.id = ur.role_id
    WHERE ur.user_id = $1 AND r.is_superuser_role = true
);
