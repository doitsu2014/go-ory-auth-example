-- name: DeleteExpiredIdempotencyKeyFor :exec
DELETE FROM idempotency_key
WHERE actor_identity_id = @actor_identity_id AND key = @key AND created_at <= @not_before;

-- name: ReserveIdempotencyKey :execrows
-- A pending reservation has response_code 0.
INSERT INTO idempotency_key (key, actor_identity_id, request_hash, response_code, response_body)
VALUES (@key, @actor_identity_id, @request_hash, 0, '{}'::jsonb)
ON CONFLICT (actor_identity_id, key) DO NOTHING;

-- name: GetIdempotencyKey :one
SELECT request_hash, response_code, response_body, created_at
FROM idempotency_key
WHERE actor_identity_id = @actor_identity_id AND key = @key;

-- name: CompleteIdempotencyKey :execrows
UPDATE idempotency_key
SET response_code = @response_code, response_body = @response_body
WHERE actor_identity_id = @actor_identity_id AND key = @key AND response_code = 0;

-- name: DeleteIdempotencyKey :exec
DELETE FROM idempotency_key WHERE actor_identity_id = @actor_identity_id AND key = @key;

-- name: DeleteExpiredIdempotencyKeys :execrows
DELETE FROM idempotency_key WHERE created_at <= @not_before;
