# Technical Specification — Customer name as encrypted PII

## 1. Data

Migration `0005_customer_pii_name.sql` (goose):
```sql
ALTER TABLE customer_pii ADD COLUMN name_ct bytea;
GRANT UPDATE (name_ct) ON customer_pii TO identity_app;
-- Down: REVOKE UPDATE (name_ct) ...; ALTER TABLE customer_pii DROP COLUMN name_ct;
```
sqlc: add `name_ct` to `UpsertCustomerPII`, `GetCustomerPII` and any list/lookup
query that returns PII rows; new `SetCustomerPIIName`:
```sql
-- name: SetCustomerPIIName :execrows
INSERT INTO customer_pii (identity_id, key_id, name_ct, updated_at)
VALUES (@identity_id, @key_id, @name_ct, now())
ON CONFLICT (identity_id) DO UPDATE SET name_ct = EXCLUDED.name_ct, updated_at = now()
WHERE customer_pii.key_id = EXCLUDED.key_id AND customer_pii.name_ct IS NULL;
```
0 rows → treat as "already has a name / key changed" and re-check.

## 2. Domain (`internal/domain/pii`)

- `FieldName Field = "name"`, column `name_ct`; `ParseField("name")`.
- `type Name struct{ First, Last string }` + redaction methods; `PersonalInfo.Name *Name`.
- Normalize: trim; both empty → nil. Validate: ≤ 100 runes each, `profile.HasControlChars` false, no NUL.
- `Masked.Name *MaskedName{First, Last *string}`; `maskPart(s)` = first rune + `***`, nil when empty.
- `FieldNames()`/`Only()` include name.

## 3. App (`internal/app/personalinfo.go`)

- Seal/open `name` like other columns (AAD binds the field name `name`; *superseded wording "column name_ct" — see DD1*).
- Lookup: after an exact phone match, open name with the others.
- New `MigrateKratosNames(ctx, dryRun bool) (MigrationReport, error)` in a new `app/namemigration.go`:
  pages `Identities.ListCustomers` (Kratos admin list, schema `customer`) — needs the
  Kratos adapter to expose `traits.name` on the admin list model (it already parses it).
  Per identity (ids only in logs): erased? (audit lookup by target + action) → strip;
  else `GetCustomerPII` has `name_ct` → strip; else seal name under `keyFor(id)` and in
  one tx: `appendAudit(customer.pii.name_migrated, actor system, details {fields:["name"]})`
  + `SetCustomerPIIName`; COMMIT; then strip. Report {scanned, migrated, stripped_only, failed}.
- Kratos port: `RemoveTraitName(ctx, id, old identity.Name) error` — *revised (DD5)*: re-read,
  compare `traits.name` with `old` (different → conflict, keep), then
  `PATCH [{"op":"remove","path":"/traits/name"}]`; re-read and retry once on 400/409.
  Kratos v26.2.0 rejects the JSON Patch `test` op.
- *Added in review:* the store transaction re-checks the erasure ledger after `SetName`
  (an erase that committed meanwhile → roll back, strip only); strip-only branches append
  `customer.pii.name_trait_removed` (reason); `EraseMine` with no key but a legacy name
  trait appends `customer.pii.erased` and removes the trait; `--strip-invalid` CLI flag;
  name parts reject Unicode format characters (Cf).
- `/v1/me` and admin customer list/detail: stop filling `Name` for customers.

## 4. CLI

`identity-service pii migrate-kratos-names [--dry-run]` → prints
`scanned=N migrated=N stripped_only=N failed=N`; exit 1 if failed > 0.

## 5. Kratos

`deploy/ory/kratos/identity-schemas/customer.v1.json` → remove `name`
(keep `$id`, bump title/description to note v1.1). `admin.v1.json` unchanged.

## 6. Clients

- Mobile: sign_up_screen drops `traits.name.*`; personal_info models/validation/screen add
  first/last (same rune/C1/Cf rules, ≤ 100); *revised in review:* the profile screen does
  not show the real name (it would fetch all PII onto an unprotected screen) — only the
  protected personal-info screen does;
  l10n vi/en; tests.
- Admin web: regenerate API types; PersonalInfoCard + lookup results show masked name
  (vi order: last first); RevealDialog field list gains name; remove the Kratos name row
  from CustomerDetailPage; tests; i18n.

## 7. Dev / seeds / smoke

- `scripts/seed-customers.mjs`: no `traits.name`; `name` in PUT; display_name `Khách hàng 0N`.
- `dev up`: after seeding the admin, run `exec identity-service /identity-service pii migrate-kratos-names` (before seeding customers).
- `scripts/smoke.mjs`: register without name; PUT with name; raw `name_ct` row has no plaintext;
  Kratos admin GET identity has no `traits.name`. (Masked-name format is covered by the
  e2e and httpapi tests instead of smoke — admin calls need an AAL2 browser session.)

## 8. Docs

08 §8.1 classification (name row → encrypted; Kratos row → email only + controls),
§8.x migration; 04-data (`name_ct`); 06-security accepted risk "email plaintext in Kratos"
(controls: Kratos DB isolated per component and role, KMS-encrypted volumes and
backups, courier/flow cleanup via `kratos cleanup sql`, no other component reads Kratos tables;
owner: platform lead); API guide; ADR-0011 amendment note; README seed note.
