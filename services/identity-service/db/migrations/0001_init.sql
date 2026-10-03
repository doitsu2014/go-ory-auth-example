-- +goose Up
-- identity database v1 (docs/architecture/04-data.md §4.4).
CREATE TABLE profile (
  identity_id   uuid        PRIMARY KEY,               -- Kratos identity id
  kind          text        NOT NULL CHECK (kind IN ('customer','admin')),
  display_name  text        CHECK (char_length(display_name) <= 100),
  avatar_url    text        CHECK (char_length(avatar_url) <= 2048),
  locale        text        NOT NULL DEFAULT 'vi-VN' CHECK (char_length(locale) <= 35),
  preferences   jsonb       NOT NULL DEFAULT '{}'::jsonb,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  deleted_at    timestamptz
);
CREATE INDEX profile_kind_created_idx ON profile (kind, created_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE audit_event (
  id                bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  occurred_at       timestamptz NOT NULL DEFAULT now(),
  actor_identity_id uuid        NOT NULL,
  action            text        NOT NULL,
  target_type       text        NOT NULL,
  target_id         text        NOT NULL,
  request_id        text        NOT NULL,
  client_ip         inet,
  details           jsonb       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX audit_event_occurred_idx ON audit_event (occurred_at DESC, id DESC);
CREATE INDEX audit_event_target_idx   ON audit_event (target_type, target_id, occurred_at DESC);

CREATE TABLE idempotency_key (
  key               text        NOT NULL,
  actor_identity_id uuid        NOT NULL,
  request_hash      bytea       NOT NULL,
  response_code     int         NOT NULL,
  response_body     jsonb       NOT NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (actor_identity_id, key)
);
CREATE INDEX idempotency_key_created_idx ON idempotency_key (created_at);

GRANT SELECT, INSERT, UPDATE ON profile TO identity_app;
GRANT SELECT, INSERT ON audit_event TO identity_app;
GRANT SELECT, INSERT, DELETE ON idempotency_key TO identity_app;

-- +goose Down
DROP TABLE idempotency_key;
DROP TABLE audit_event;
DROP TABLE profile;
