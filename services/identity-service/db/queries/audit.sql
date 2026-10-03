-- name: InsertAuditEvent :one
INSERT INTO audit_event (actor_identity_id, action, target_type, target_id, request_id, client_ip, details)
VALUES (@actor_identity_id, @action, @target_type, @target_id, @request_id, sqlc.narg(client_ip), @details)
RETURNING id, occurred_at;

-- name: ListAuditEvents :many
-- Keyset pagination on (occurred_at, id), newest first.
SELECT id, occurred_at, actor_identity_id, action, target_type, target_id, request_id, details
FROM audit_event
WHERE (sqlc.narg(target_type)::text IS NULL OR target_type = sqlc.narg(target_type)::text)
  AND (sqlc.narg(target_id)::text IS NULL OR target_id = sqlc.narg(target_id)::text)
  AND (sqlc.narg(actor_id)::uuid IS NULL OR actor_identity_id = sqlc.narg(actor_id)::uuid)
  AND (sqlc.narg(after_occurred_at)::timestamptz IS NULL
       OR (occurred_at, id) < (sqlc.narg(after_occurred_at)::timestamptz, sqlc.narg(after_id)::bigint))
ORDER BY occurred_at DESC, id DESC
LIMIT @page_limit;
