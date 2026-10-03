-- +goose Up
-- Keyset-friendly target index (id tiebreaker) and an actor index for the
-- audit list filters; UPDATE on idempotency_key for pending reservations.
DROP INDEX audit_event_target_idx;
CREATE INDEX audit_event_target_idx ON audit_event (target_type, target_id, occurred_at DESC, id DESC);
CREATE INDEX audit_event_actor_idx ON audit_event (actor_identity_id, occurred_at DESC, id DESC);
GRANT UPDATE ON idempotency_key TO identity_app;

-- +goose Down
REVOKE UPDATE ON idempotency_key FROM identity_app;
DROP INDEX audit_event_actor_idx;
DROP INDEX audit_event_target_idx;
CREATE INDEX audit_event_target_idx ON audit_event (target_type, target_id, occurred_at DESC);
