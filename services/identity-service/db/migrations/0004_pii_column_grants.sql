-- +goose Up
-- Column-level UPDATE for the app role (review hardening): the identity
-- binding (identity_id, created_at, and subject_key.key_id) can never be
-- rewritten by identity_app, only the encrypted data and key-wrapping state.
REVOKE UPDATE ON customer_pii FROM identity_app;
GRANT UPDATE (key_id, phone_ct, phone_bidx, bidx_key_version, dob_ct, address_ct, national_id_ct, updated_at)
  ON customer_pii TO identity_app;

REVOKE UPDATE ON subject_key FROM identity_app;
GRANT UPDATE (wrapped_dek, kek_name, kek_version, rewrapped_at) ON subject_key TO identity_app;

-- +goose Down
REVOKE UPDATE (wrapped_dek, kek_name, kek_version, rewrapped_at) ON subject_key FROM identity_app;
GRANT UPDATE ON subject_key TO identity_app;
REVOKE UPDATE (key_id, phone_ct, phone_bidx, bidx_key_version, dob_ct, address_ct, national_id_ct, updated_at)
  ON customer_pii FROM identity_app;
GRANT UPDATE ON customer_pii TO identity_app;
