# Technical Specification — Encrypted personal information

Builds on the 261003-t-i-mu spec (Go 1.25, chi, oapi-codegen strict server,
pgx/v5, sqlc, goose, hand-written HTTP adapters, hexagonal layout).

## 1. Database — migration `0003_customer_pii.sql` (goose, migrator role)

```sql
CREATE TABLE subject_key (
  key_id       uuid        PRIMARY KEY,                 -- random per DEK, app-generated
  identity_id  uuid        NOT NULL UNIQUE,             -- Kratos customer id
  wrapped_dek  text        NOT NULL CHECK (char_length(wrapped_dek) <= 512),
  kek_name     text        NOT NULL CHECK (char_length(kek_name) <= 128),
  kek_version  int         NOT NULL CHECK (kek_version > 0),
  created_at   timestamptz NOT NULL DEFAULT now(),
  rewrapped_at timestamptz
);
CREATE INDEX subject_key_kek_version_idx ON subject_key (kek_name, kek_version);

CREATE TABLE customer_pii (
  identity_id      uuid        PRIMARY KEY REFERENCES subject_key (identity_id) ON DELETE CASCADE,
  key_id           uuid        NOT NULL REFERENCES subject_key (key_id) ON DELETE CASCADE,
  phone_ct         bytea,
  phone_bidx       bytea       CHECK (phone_bidx IS NULL OR octet_length(phone_bidx) = 32),
  bidx_key_version int,
  dob_ct           bytea,
  address_ct       bytea,
  national_id_ct   bytea,
  updated_at       timestamptz NOT NULL DEFAULT now(),
  CHECK ((phone_ct IS NULL) = (phone_bidx IS NULL))
);
CREATE INDEX customer_pii_phone_bidx_idx ON customer_pii (phone_bidx) WHERE phone_bidx IS NOT NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON subject_key  TO identity_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON customer_pii TO identity_app;
```

Queries (`db/queries/pii.sql`, sqlc): `GetSubjectKey(identity_id)`,
`InsertSubjectKey … ON CONFLICT (identity_id) DO NOTHING RETURNING *`,
`DeleteSubjectKey(identity_id)`, `UpsertCustomerPII`, `GetCustomerPII`,
`FindCustomerPIIByPhoneBidx(bidx) LIMIT 20`, `ListSubjectKeysForRewrap(after key_id, limit)`,
`UpdateWrappedDEK(key_id, old, new, version)`.

## 2. Go packages

| Package | Contents |
| --- | --- |
| `internal/domain/pii` | `PersonalInfo{Phone *string; DateOfBirth *civil date; Address *Address; NationalID *NationalID}`, `Normalize()`, `Validate(now) error` (returns `*profile.ValidationError`-style field errors), `Mask() Masked`, `FieldNames()`, `LogValue()` → `[REDACTED]` |
| `internal/crypto/envelope` | `NewDEK() ([]byte, error)`, `Seal(dek, aad, pt) ([]byte, error)`, `Open(dek, aad, ct) ([]byte, error)`, `AAD(column string, id uuid.UUID) []byte`, `Zero([]byte)`; format `0x01‖nonce‖ct‖tag`; `ErrDecrypt` |
| `internal/app` (ports) | `KeyManager{ WrapDEK(ctx, dek) (Wrapped, error); UnwrapDEK(ctx, Wrapped) ([]byte, error); RewrapDEK(ctx, Wrapped) (Wrapped, error); BlindIndex(ctx, input []byte) (BlindIndex, error) }` with `Wrapped{Ciphertext, KEKName string; KEKVersion int}`, `BlindIndex{Sum []byte; KeyVersion int}`; repos `SubjectKeyRepo`, `CustomerPIIRepo` added to `Repos` (tx-aware) |
| `internal/app/personalinfo.go` | `PersonalInfoService`: `GetMine`, `PutMine`, `EraseMine`, `GetMasked`, `Reveal`, `LookupByPhone`; `DEKCache` (LRU+TTL, `Evict(keyID)`) in `internal/app/dekcache.go` |
| `internal/adapter/openbao` | Transit client: `POST /v1/transit/{encrypt,decrypt,rewrap}/<kek>`, `/v1/transit/hmac/<bidx>/sha2-256`; header `X-Vault-Token` from token file (re-read on change); `RenewLoop(ctx)` via `/v1/auth/token/renew-self` every period/2; errors → `app.ErrDependencyUnavailable`; never logs request/response bodies |
| `internal/adapter/localkms` | AES-256-GCM wrap with a local KEK, HMAC-SHA256 with a local index key; for tests and `PII_KMS_PROVIDER=local` (refused when `APP_ENV=production`) |
| `internal/adapter/httpapi` | handlers for the 6 operations, route policies (`/v1/me/personal-info` self; admin routes Keto permits), problem mapping |
| `cmd/identity-service` | `keys rewrap [--batch 100]` subcommand |

AAD: `"identity-service/customer_pii/" + column + "/" + identity_id + "/v1"`;
columns `phone_number`, `date_of_birth`, `address`, `national_id`. Address and
national id are JSON-encoded before sealing. Blind index input:
`"phone_number:" + E.164`.

## 3. Config (env)

| Var | Default | Notes |
| --- | --- | --- |
| `PII_KMS_PROVIDER` | `openbao` | `openbao` \| `local` |
| `PII_OPENBAO_ADDR` | `http://127.0.0.1:8200` | |
| `PII_OPENBAO_TOKEN_FILE` | — | required for `openbao` |
| `PII_OPENBAO_KEK_NAME` | `identity-pii-kek` | |
| `PII_OPENBAO_BIDX_KEY_NAME` | `identity-pii-bidx` | |
| `PII_OPENBAO_TIMEOUT` | `2s` | |
| `PII_LOCAL_KEK` / `PII_LOCAL_BIDX_KEY` | — | base64 32 bytes, `local` only |
| `PII_DEK_CACHE_TTL` / `PII_DEK_CACHE_SIZE` | `5m` / `10000` | |

## 4. Keto

`namespaces.keto.ts`: `reveal_customer_pii: (ctx) => this.permits.manage_customers(ctx)`.
`Permission` list in the service and the `/admin/v1/me` mapping gain it.

## 5. Compose

New: `deploy/openbao/openbao.hcl`, `deploy/openbao/init.sh`,
`deploy/openbao/policies/{identity-service,identity-pii-operator}.hcl`; services
`openbao` (healthcheck `bao status` exit 0/2 → reachable) and `openbao-init`
(one-shot; `service_completed_successfully` dependency for identity-service);
volumes `openbao-data`, `openbao-keys`, `openbao-app-token`. Make targets:
`kek-rotate` (operator token), `keys-rewrap` (runs the CLI in the service container).
`.env.example` gains nothing secret: the token is produced at runtime.

## 6. Logging / redaction

- PII types are `LogValuer`s that print `[REDACTED]`.
- The request logger never logs bodies (already true); add a test that
  captures logs through a PUT/GET/reveal and asserts none of the test values
  appear.
- OpenBao adapter logs operation + status only.

## 7. Clients

- **Mobile** (`apps/mobile`): `PersonalInfo` model, `IdentityClient` methods,
  `PersonalInfoScreen` (view/edit/erase), route `/profile/personal-info`, vi/en
  strings, validation mirroring the server, 503 → retry banner.
- **Admin web** (`apps/admin-web`): `PersonalInfoCard` on customer detail
  (masked + "Reveal" for `reveal_customer_pii`), `RevealDialog` (reason,
  10–200 chars), revealed data kept in component state only (not the query
  cache), cleared on unmount / 60 s timer; phone lookup form on the customers
  page (POST); i18n vi/en; MSW tests.

## 8. Testing

| Level | What |
| --- | --- |
| Unit | envelope (round trip, wrong AAD, tamper, wrong key, version byte), domain validation/normalisation/masking tables, DEK cache TTL/evict, localkms, service with fakes |
| Integration (`-tags integration`) | real Postgres + OpenBao: raw rows contain no plaintext; AAD swap fails; erase → key row gone and old ciphertext undecryptable; rotate + rewrap keeps data readable; lookup; reveal audit atomicity; OpenBao stopped → 503 |
| Smoke | `scripts/smoke.mjs` +4 checks: PUT/GET own PII, masked admin view, reveal audited, erase |

## 9. Security design review — resolutions (security-agent, 2026-10-03)

These override anything above that disagrees.

| # | Finding | Resolution |
| --- | --- | --- |
| B1 | Crypto-shred vs backups/PITR | Accepted risk in 06-security §6.7 (owner: identity-service maintainers). KEK rotation cadence ≤ backup retention, then `min_decryption_version` + `trim`. **Erasure ledger**: CLI `identity-service pii reapply-erasures` deletes `subject_key` for every `customer.pii.erased` audit target; runbook says run it after any restore. Deadlines documented (GDPR 1 month; Decree 13 Art. 16 72 h) |
| B2 | Free-text reveal reason → PII in append-only audit | `RevealRequest{reason_code enum, ticket_ref pattern, fields[]}`; no free text anywhere |
| B3 | Lookup enumeration | Per-actor limiter 30/min + 200/day (in-memory, per replica) → 429 `rate_limited`; audit `customer.pii.lookup` with `details.bidx` (hex of the keyed hash, never the phone) + `details.matched_ids`; results carry id + state + masked info, **no email** |
| B4 | Compliance mapping | Rewritten in docs: Decree 13 classes these as *basic* personal data (Art. 2) — cite Art. 26/27 (protection measures), 16 (deletion), 23 (72 h breach notice), 24 (impact assessment), plus the Personal Data Protection Law No. 91/2025/QH15 (effective 2026-01-01; article numbers to be confirmed by counsel). ASVS: V6.1.1, V6.2.1–6.2.6, V6.4.1–6.4.2, V8.3.1, V8.3.4–8.3.6, V4.1.x |
| B5 | OpenBao policy | Written (`deploy/openbao/policies/`): `update` on encrypt/decrypt KEK + hmac bidx; token self renew/lookup; nothing under `transit/keys/*`. Index key `auto_rotate_period=0`; hmac calls pin `key_version=1` |
| B6 | Token lifecycle | Orphan, `-no-default-policy`, `period=24h`; previous accessor revoked when a new token is issued; `openbao-keys` never mounted into the service. `bound_cidrs` and root-token revocation not done locally (documented; prod uses Kubernetes auth + auto-unseal) |
| A7 | Format byte unauthenticated | AAD = `"identity-service/pii/v1\x00" ‖ format byte ‖ column ‖ identity_id` (one version scheme) |
| A8 | Replay of older ciphertext in the same cell | Accepted and documented (writer to DB already has write access) |
| A9 | Wrapped DEK not bound to subject | Transit `associated_data` = `identity_id ‖ "/" ‖ key_id` (verified on OpenBao 2.4.1; wrong AD → `message authentication failed`). Transit `rewrap` does not support AD → `keys rewrap` = decrypt + encrypt with the same AD; `rewrap` removed from the app policy. Composite FK `customer_pii(identity_id, key_id) → subject_key(identity_id, key_id)` (add `UNIQUE (identity_id, key_id)`) |
| A10 | Swapped blind index | Lookup decrypts each candidate's phone and keeps only exact matches |
| A11 | DEK cache residency | Documented (≤ 5 min on other replicas after erase); lookup unwraps count against its limiter; max 20 candidates |
| A12 | Masking leakage | Fixed width: phone = `+<cc>` + `*******` + last 3; national id = `******` + last 3 only when length ≥ 9, else `******`; DOB year only; address city + country |
| A13 | Reveal ordering | decrypt in memory → audit COMMIT → respond; audit failure → zero buffers, 500. Per-actor quota 20/hour → 429. Optional `fields[]` |
| A14 | Erasure gated on verified email | `DELETE` allowed for any authenticated customer. Kratos-side identity deletion cleanup: out of scope (no delete endpoint exists), noted as follow-up |
| A15 | Codes / provider guards | 503 code is `dependency_unavailable` everywhere. `localkms` only when `APP_ENV ∈ {local, test}`; `http://` OpenBao address refused when `APP_ENV=production` |
| A16 | Errors logging values | PII-path errors wrapped in value-free sentinels before they reach `errorWriter` (no pgx DETAIL, parse errors or JSON values); log-capture test covers error paths |
| A17 | Grants | `identity_app`: `subject_key` SELECT/INSERT/UPDATE/DELETE; `customer_pii` SELECT/INSERT/UPDATE (rows go via the FK cascade, which runs with owner rights) |
