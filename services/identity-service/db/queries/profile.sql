-- name: InsertProfileIfAbsent :exec
INSERT INTO profile (identity_id, kind)
VALUES (@identity_id, @kind)
ON CONFLICT (identity_id) DO NOTHING;

-- name: GetProfile :one
SELECT identity_id, kind, display_name, avatar_url, locale, created_at, updated_at
FROM profile
WHERE identity_id = @identity_id AND deleted_at IS NULL;

-- name: GetProfilesByIDs :many
SELECT identity_id, kind, display_name, avatar_url, locale, created_at, updated_at
FROM profile
WHERE identity_id = ANY(@ids::uuid[]) AND deleted_at IS NULL;

-- name: UpdateProfile :one
UPDATE profile
SET display_name = @display_name,
    avatar_url   = @avatar_url,
    locale       = @locale,
    updated_at   = now()
WHERE identity_id = @identity_id AND deleted_at IS NULL
RETURNING identity_id, kind, display_name, avatar_url, locale, created_at, updated_at;
