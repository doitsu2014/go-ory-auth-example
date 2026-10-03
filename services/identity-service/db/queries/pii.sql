-- name: GetSubjectKey :one
SELECT key_id, identity_id, wrapped_dek, kek_name, kek_version, created_at, rewrapped_at
FROM subject_key
WHERE identity_id = @identity_id;

-- name: InsertSubjectKey :one
-- No row is returned when the subject already has a key (the caller re-reads).
INSERT INTO subject_key (key_id, identity_id, wrapped_dek, kek_name, kek_version)
VALUES (@key_id, @identity_id, @wrapped_dek, @kek_name, @kek_version)
ON CONFLICT (identity_id) DO NOTHING
RETURNING key_id, identity_id, wrapped_dek, kek_name, kek_version, created_at, rewrapped_at;

-- name: DeleteSubjectKey :many
-- Cascades to customer_pii (crypto-shredding).
DELETE FROM subject_key WHERE identity_id = @identity_id
RETURNING key_id;

-- name: ListSubjectKeysForRewrap :many
SELECT key_id, identity_id, wrapped_dek, kek_name, kek_version, created_at, rewrapped_at
FROM subject_key
WHERE key_id > @after_key_id
ORDER BY key_id
LIMIT @page_limit;

-- name: UpdateWrappedDEK :execrows
-- Optimistic: only if nobody changed (or erased) the key meanwhile.
UPDATE subject_key
SET wrapped_dek = @new_wrapped_dek, kek_name = @kek_name, kek_version = @kek_version, rewrapped_at = now()
WHERE key_id = @key_id AND wrapped_dek = @old_wrapped_dek;

-- name: DeleteErasedSubjectKeys :execrows
-- Erasure ledger (§9 B1): delete keys created at or before the subject's
-- latest customer.pii.erased audit event (run after any restore).
DELETE FROM subject_key k
USING (
  SELECT target_id, max(occurred_at) AS erased_at
  FROM audit_event
  WHERE action = 'customer.pii.erased' AND target_type = 'customer'
  GROUP BY target_id
) e
WHERE k.identity_id::text = e.target_id AND k.created_at <= e.erased_at;

-- name: UpsertCustomerPII :one
INSERT INTO customer_pii (identity_id, key_id, phone_ct, phone_bidx, bidx_key_version, dob_ct, address_ct, national_id_ct, name_ct, updated_at)
VALUES (@identity_id, @key_id, sqlc.narg(phone_ct), sqlc.narg(phone_bidx), sqlc.narg(bidx_key_version),
        sqlc.narg(dob_ct), sqlc.narg(address_ct), sqlc.narg(national_id_ct), sqlc.narg(name_ct), now())
ON CONFLICT (identity_id) DO UPDATE
SET key_id           = EXCLUDED.key_id,
    phone_ct         = EXCLUDED.phone_ct,
    phone_bidx       = EXCLUDED.phone_bidx,
    bidx_key_version = EXCLUDED.bidx_key_version,
    dob_ct           = EXCLUDED.dob_ct,
    address_ct       = EXCLUDED.address_ct,
    national_id_ct   = EXCLUDED.national_id_ct,
    name_ct          = EXCLUDED.name_ct,
    updated_at       = now()
RETURNING updated_at;

-- name: SetCustomerPIIName :execrows
-- Migration of a Kratos name trait (technical spec §1): adds name_ct without
-- touching the other columns. Inserts the record when there is none; else
-- fills name_ct only when the record is under the given key and has no name
-- yet. 0 rows = the record already has a name or is under another key.
INSERT INTO customer_pii (identity_id, key_id, name_ct, updated_at)
VALUES (@identity_id, @key_id, @name_ct, now())
ON CONFLICT (identity_id) DO UPDATE
SET name_ct = EXCLUDED.name_ct, updated_at = now()
WHERE customer_pii.key_id = EXCLUDED.key_id AND customer_pii.name_ct IS NULL;

-- name: GetCustomerPII :one
SELECT identity_id, key_id, phone_ct, phone_bidx, bidx_key_version, dob_ct, address_ct, national_id_ct, updated_at, name_ct
FROM customer_pii
WHERE identity_id = @identity_id;

-- name: FindCustomerPIIByPhoneBidx :many
SELECT identity_id, key_id, phone_ct, phone_bidx, bidx_key_version, dob_ct, address_ct, national_id_ct, updated_at, name_ct
FROM customer_pii
WHERE phone_bidx = @phone_bidx AND bidx_key_version = @bidx_key_version
ORDER BY identity_id
LIMIT @page_limit;
