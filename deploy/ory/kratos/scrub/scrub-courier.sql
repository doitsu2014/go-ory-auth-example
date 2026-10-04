-- Kratos courier retention and post-migration scrub (ADR-0013, PLI-FR-14).
--
-- `kratos cleanup sql` never deletes courier messages (spike S5), and before
-- the login migration they hold customers' plaintext email addresses (the
-- recipient, the rendered body and template_data). This operator script runs
-- with the kratos database role, never from identity-service (which must not
-- touch Kratos tables).
--
--   psql "$KRATOS_DSN" -v keep_days=7 -v phase=transition -f scrub-courier.sql
--
-- keep_days: messages older than this are deleted (retention; integer).
-- phase:     'complete' additionally deletes every message whose recipient is
--            neither a pseudonym nor an admin's address, i.e. the legacy
--            customer messages. Run it once after migrate-kratos-logins.
--
-- Other legacy plaintext after the migration lives in self-service flows
-- (`ui` JSON holds typed identifiers) and verification/recovery code rows;
-- `kratos cleanup sql --keep-last <d>` removes them once expired. Run the
-- final scrub at least 24 h after the migration, or with KEEP_LAST=1h
-- (`make kratos-scrub PHASE=complete KEEP_LAST=1h`).
\set ON_ERROR_STOP on
BEGIN;

CREATE TEMP TABLE scrub_target ON COMMIT DROP AS
SELECT m.id
FROM courier_messages m
WHERE m.created_at < now() - make_interval(days => (:'keep_days')::int)
   OR (:'phase' = 'complete'
       AND m.recipient NOT LIKE '%@login.invalid'
       AND NOT EXISTS (
         SELECT 1
         FROM identity_verifiable_addresses a
         JOIN identities i ON i.id = a.identity_id AND i.nid = a.nid
         WHERE i.schema_id = 'admin' AND lower(a.value) = lower(m.recipient)
       ));

DELETE FROM courier_message_dispatches WHERE message_id IN (SELECT id FROM scrub_target);
DELETE FROM courier_messages WHERE id IN (SELECT id FROM scrub_target);
SELECT count(*) AS courier_messages_deleted FROM scrub_target;

COMMIT;
