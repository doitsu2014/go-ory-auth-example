-- +goose Up
-- Customer real name as encrypted personal information (intent
-- 261003-move-customer-full, technical spec §1). The name moves out of the
-- Kratos traits (plaintext) into a per-column AES-256-GCM ciphertext under
-- the customer's DEK. Like every other column, the AAD binds the field name
-- ("name"), not the column name. Nullable: existing records keep decrypting
-- (NAME-NFR-02).
ALTER TABLE customer_pii ADD COLUMN name_ct bytea;
GRANT UPDATE (name_ct) ON customer_pii TO identity_app;

-- +goose Down
REVOKE UPDATE (name_ct) ON customer_pii FROM identity_app;
ALTER TABLE customer_pii DROP COLUMN name_ct;
