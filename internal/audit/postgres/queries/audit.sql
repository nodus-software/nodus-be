-- name: InsertAuditLog :exec
INSERT INTO audit_logs (id, "timestamp", user_id, action, target_resource, ip_address, result, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
