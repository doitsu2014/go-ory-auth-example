-- +goose Up
-- Customer login through identity-service (intent 261004-refactor-customer-login,
-- ADR-0014). The Kratos identifier (login_identifier.pseudonym, the handle in
-- traits.login_id) is decoupled from the keyed hash of the address:
-- lookup_key is HMAC(address) and finds the row at sign-in; new handles are
-- random. Rows created before this migration used the HMAC as their handle,
-- so the backfill is lookup_key = pseudonym and no Kratos identity changes.
-- Re-keying the HMAC now rewrites lookup_key only (the AAD binds the
-- ciphertext to the handle, which stays).
ALTER TABLE login_identifier ADD COLUMN lookup_key bytea;
UPDATE login_identifier SET lookup_key = pseudonym;
ALTER TABLE login_identifier
  ALTER COLUMN lookup_key SET NOT NULL,
  ADD CONSTRAINT login_identifier_lookup_key_len CHECK (octet_length(lookup_key) = 32),
  ADD CONSTRAINT login_identifier_lookup_key_key UNIQUE (lookup_key);

-- +goose Down
ALTER TABLE login_identifier DROP COLUMN lookup_key;
