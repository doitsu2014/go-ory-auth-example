-- +goose Up
-- Pseudonymous customer login identifiers (intent
-- 261004-pseudonymize-customer-login, ADR-0013). Kratos stores only the
-- pseudonym "<base32(hmac)>@login.invalid"; the email address or phone
-- number lives here, encrypted with the login KEK in OpenBao Transit and
-- bound (associated data) to its kind and pseudonym.
CREATE TABLE login_identifier (
  pseudonym         bytea       PRIMARY KEY CHECK (octet_length(pseudonym) = 32), -- raw HMAC-SHA256
  kind              text        NOT NULL CHECK (kind IN ('email', 'phone')),
  value_ct          text        NOT NULL CHECK (char_length(value_ct) <= 2048),
  kek_version       int         NOT NULL CHECK (kek_version > 0),
  identity_id       uuid        UNIQUE,              -- Kratos customer id; NULL until bound
  bound_at          timestamptz,
  legacy_verified   boolean     NOT NULL DEFAULT false, -- migrated login was verified (S4b)
  created_at        timestamptz NOT NULL DEFAULT now(),
  last_validated_at timestamptz NOT NULL DEFAULT now(), -- pre-registration check (A10)
  CONSTRAINT login_identifier_bound_ck CHECK ((identity_id IS NULL) = (bound_at IS NULL))
);
CREATE INDEX login_identifier_unbound_idx ON login_identifier (last_validated_at) WHERE identity_id IS NULL;

-- Courier de-duplication (A9): keyed hash of (template, recipient, code).
-- Delivered rows also carry the channel, the SMS calling code and a keyed
-- hash of the recipient, so the per-recipient quota and the SMS budgets are
-- durable across restarts and shared by replicas (A3, SEC-C02).
CREATE TABLE courier_dispatch (
  dedupe_key    bytea       PRIMARY KEY CHECK (octet_length(dedupe_key) = 32),
  state         text        NOT NULL CHECK (state IN ('pending', 'sent')),
  channel       text        CHECK (channel IN ('email', 'sms')),
  country       text        CHECK (country ~ '^[0-9]{1,3}$'),
  recipient_key bytea       CHECK (octet_length(recipient_key) = 32),
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX courier_dispatch_created_idx ON courier_dispatch (created_at);
CREATE INDEX courier_dispatch_recipient_idx ON courier_dispatch (recipient_key, updated_at) WHERE channel IS NOT NULL;
CREATE INDEX courier_dispatch_sms_idx ON courier_dispatch (updated_at, country) WHERE channel = 'sms';

-- The pseudonym, kind and creation time of a login are never rewritten by the
-- app role; only the binding, verification flag, validation time and the
-- (re-wrapped) ciphertext change.
GRANT SELECT, INSERT, DELETE ON login_identifier TO identity_app;
GRANT UPDATE (identity_id, bound_at, legacy_verified, last_validated_at, value_ct, kek_version)
  ON login_identifier TO identity_app;
GRANT SELECT, INSERT, DELETE ON courier_dispatch TO identity_app;
GRANT UPDATE (state, channel, country, recipient_key, updated_at) ON courier_dispatch TO identity_app;

-- +goose Down
DROP TABLE courier_dispatch;
DROP TABLE login_identifier;
