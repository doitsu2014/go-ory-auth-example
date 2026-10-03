-- +goose Up
-- Encrypted customer personal information (intent 261003-encrypt-user-pii,
-- technical spec §1 + §9 A9/A17). PostgreSQL holds only ciphertext, wrapped
-- data keys and keyed hashes; plaintext never reaches the database.

-- One data encryption key (DEK) per customer, wrapped by the KEK in OpenBao
-- Transit with associated data identity_id/key_id. Deleting the row
-- crypto-shreds the customer's personal information.
CREATE TABLE subject_key (
  key_id       uuid        PRIMARY KEY,                 -- random per DEK, app-generated
  identity_id  uuid        NOT NULL UNIQUE,             -- Kratos customer id
  wrapped_dek  text        NOT NULL CHECK (char_length(wrapped_dek) <= 512),
  kek_name     text        NOT NULL CHECK (char_length(kek_name) <= 128),
  kek_version  int         NOT NULL CHECK (kek_version > 0),
  created_at   timestamptz NOT NULL DEFAULT now(),
  rewrapped_at timestamptz,
  CONSTRAINT subject_key_identity_key_uq UNIQUE (identity_id, key_id)
);
CREATE INDEX subject_key_kek_version_idx ON subject_key (kek_name, kek_version);

-- Per-column AES-256-GCM ciphertexts (0x01 ‖ nonce ‖ ct ‖ tag). The
-- composite FK binds a record to its subject's current key and removes it
-- when the key is deleted.
CREATE TABLE customer_pii (
  identity_id      uuid        PRIMARY KEY,
  key_id           uuid        NOT NULL,
  phone_ct         bytea,
  phone_bidx       bytea       CHECK (phone_bidx IS NULL OR octet_length(phone_bidx) = 32),
  bidx_key_version int         CHECK (bidx_key_version IS NULL OR bidx_key_version > 0),
  dob_ct           bytea,
  address_ct       bytea,
  national_id_ct   bytea,
  updated_at       timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT customer_pii_phone_bidx_ck CHECK ((phone_ct IS NULL) = (phone_bidx IS NULL)),
  CONSTRAINT customer_pii_bidx_version_ck CHECK ((phone_bidx IS NULL) = (bidx_key_version IS NULL)),
  CONSTRAINT customer_pii_subject_key_fk FOREIGN KEY (identity_id, key_id)
    REFERENCES subject_key (identity_id, key_id) ON DELETE CASCADE
);
CREATE INDEX customer_pii_phone_bidx_idx ON customer_pii (phone_bidx, bidx_key_version) WHERE phone_bidx IS NOT NULL;

-- §9 A17: no DELETE on customer_pii for the app role; records go only via
-- the FK cascade from subject_key (crypto-shredding).
GRANT SELECT, INSERT, UPDATE, DELETE ON subject_key  TO identity_app;
GRANT SELECT, INSERT, UPDATE         ON customer_pii TO identity_app;

-- +goose Down
DROP TABLE customer_pii;
DROP TABLE subject_key;
