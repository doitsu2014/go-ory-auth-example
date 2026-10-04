-- name: InsertLoginIdentifier :execrows
-- Registration resolve (PLI-FR-02): idempotent insert.
INSERT INTO login_identifier (pseudonym, kind, value_ct, kek_version, identity_id, bound_at, legacy_verified)
VALUES (@pseudonym, @kind, @value_ct, @kek_version, sqlc.narg(identity_id),
        CASE WHEN sqlc.narg(identity_id)::uuid IS NULL THEN NULL ELSE now() END, @legacy_verified)
ON CONFLICT (pseudonym) DO NOTHING;

-- name: GetLoginIdentifier :one
SELECT pseudonym, kind, value_ct, kek_version, identity_id, bound_at, legacy_verified, created_at, last_validated_at
FROM login_identifier
WHERE pseudonym = @pseudonym;

-- name: GetLoginIdentifiers :many
SELECT pseudonym, kind, value_ct, kek_version, identity_id, bound_at, legacy_verified, created_at, last_validated_at
FROM login_identifier
WHERE pseudonym = ANY(@pseudonyms::bytea[]);

-- name: GetLoginIdentifierByIdentity :one
SELECT pseudonym, kind, value_ct, kek_version, identity_id, bound_at, legacy_verified, created_at, last_validated_at
FROM login_identifier
WHERE identity_id = @identity_id;

-- name: BindLoginIdentifier :execrows
-- Binds when unbound or already bound to the same identity; with
-- stale_identity it also replaces exactly that binding (the caller verified
-- that identity is gone), never a binding made meanwhile.
UPDATE login_identifier
SET identity_id = @identity_id, bound_at = COALESCE(CASE WHEN identity_id = @identity_id THEN bound_at END, now())
WHERE pseudonym = @pseudonym
  AND (identity_id IS NULL OR identity_id = @identity_id OR identity_id = sqlc.narg(stale_identity));

-- name: SetLoginLegacyVerified :execrows
UPDATE login_identifier SET legacy_verified = @legacy_verified WHERE pseudonym = @pseudonym;

-- name: TouchLoginIdentifier :execrows
UPDATE login_identifier SET last_validated_at = now() WHERE pseudonym = @pseudonym;

-- name: ListStaleUnboundLoginIdentifiers :many
-- Purge (A10): unbound rows not validated since @before.
SELECT pseudonym, kind, value_ct, kek_version, identity_id, bound_at, legacy_verified, created_at, last_validated_at
FROM login_identifier
WHERE identity_id IS NULL AND last_validated_at < @before
ORDER BY last_validated_at
LIMIT @page_limit;

-- name: DeleteStaleUnboundLoginIdentifier :execrows
-- Purge (A10): deletes only while still unbound and not validated since
-- @before, so a registration that validated the row meanwhile keeps it.
DELETE FROM login_identifier
WHERE pseudonym = @pseudonym AND identity_id IS NULL AND last_validated_at < @before;

-- name: ListBoundLoginIdentifiers :many
SELECT pseudonym, kind, value_ct, kek_version, identity_id, bound_at, legacy_verified, created_at, last_validated_at
FROM login_identifier
WHERE identity_id IS NOT NULL AND pseudonym > @after
ORDER BY pseudonym
LIMIT @page_limit;

-- name: ListAllLoginIdentifiers :many
SELECT pseudonym, kind, value_ct, kek_version, identity_id, bound_at, legacy_verified, created_at, last_validated_at
FROM login_identifier
WHERE pseudonym > @after
ORDER BY pseudonym
LIMIT @page_limit;

-- name: DeleteLoginIdentifier :execrows
DELETE FROM login_identifier
WHERE pseudonym = @pseudonym AND (NOT @only_if_unbound::boolean OR identity_id IS NULL);

-- name: DeleteLoginIdentifierForIdentity :execrows
DELETE FROM login_identifier WHERE identity_id = @identity_id;

-- name: UpdateLoginCiphertext :execrows
-- Re-wrap: optimistic, only if nobody changed (or deleted) the row meanwhile.
UPDATE login_identifier SET value_ct = @new_value_ct, kek_version = @kek_version
WHERE pseudonym = @pseudonym AND value_ct = @old_value_ct;

-- name: CountUnboundLoginIdentifiers :one
SELECT count(*) FROM login_identifier WHERE identity_id IS NULL;

-- name: DeleteErasedLoginIdentifiers :execrows
-- Erasure ledger: logins of identities with a customer.login.erased event
-- (re-applied after a restore).
DELETE FROM login_identifier l
USING (
  SELECT DISTINCT target_id
  FROM audit_event
  WHERE action = 'customer.login.erased' AND target_type = 'customer'
) e
WHERE l.identity_id::text = e.target_id;

-- name: ReserveCourierDispatch :execrows
-- A9: claim a key; a stale pending reservation (crashed sender) is retaken.
INSERT INTO courier_dispatch (dedupe_key, state) VALUES (@dedupe_key, 'pending')
ON CONFLICT (dedupe_key) DO UPDATE SET updated_at = now()
WHERE courier_dispatch.state = 'pending' AND courier_dispatch.updated_at < @stale_before;

-- name: MarkCourierDispatchSent :execrows
-- Delivered messages record channel/country/recipient (quota counting);
-- permanently dropped ones are marked sent with NULLs (not counted).
UPDATE courier_dispatch
SET state = 'sent', channel = sqlc.narg(channel), country = sqlc.narg(country),
    recipient_key = sqlc.narg(recipient_key), updated_at = now()
WHERE dedupe_key = @dedupe_key;

-- name: CountCourierDeliveriesTo :one
SELECT count(*) FROM courier_dispatch
WHERE recipient_key = @recipient_key AND channel IS NOT NULL AND updated_at >= @since;

-- name: CountSMSDeliveries :one
-- country '' = every country.
SELECT count(*) FROM courier_dispatch
WHERE channel = 'sms' AND updated_at >= @since AND (@country::text = '' OR country = @country::text);

-- name: ReleaseCourierDispatch :execrows
DELETE FROM courier_dispatch WHERE dedupe_key = @dedupe_key AND state = 'pending';

-- name: PurgeCourierDispatch :execrows
DELETE FROM courier_dispatch WHERE created_at < @before;
