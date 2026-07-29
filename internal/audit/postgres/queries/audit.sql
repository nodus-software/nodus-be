-- name: InsertAuditLog :exec
INSERT INTO audit_logs (id, "timestamp", user_id, action, target_resource, ip_address, result, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, sqlc.arg(metadata)::text::jsonb);

-- name: ListAuditLogs :many
SELECT * FROM audit_logs
WHERE (sqlc.narg(user_id)::uuid IS NULL OR user_id = sqlc.narg(user_id))
  AND (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action))
  AND (sqlc.narg(from_ts)::timestamptz IS NULL OR "timestamp" >= sqlc.narg(from_ts))
  AND (sqlc.narg(to_ts)::timestamptz IS NULL OR "timestamp" <= sqlc.narg(to_ts))
ORDER BY "timestamp" DESC
LIMIT $1;
