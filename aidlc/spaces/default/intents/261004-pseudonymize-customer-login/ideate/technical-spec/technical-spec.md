# Technical Specification — Pseudonymous customer login identifiers

Inputs: requirements (PLI-*), architecture-doc (§1–§10, amendments A1–A14),
api-contract (I1–I14), design-decisions (DD-01..19).

Stack (unchanged, recorded per db-postgres skill): PostgreSQL 16, Go `pgx/v5`
+ `pgxpool`, `sqlc`, `goose` (embedded SQL, forward-only), roles
`identity_migrator` (DDL) / `identity_app` (DML, column grants). Kratos
v26.2.0, OpenBao 2.4.1, Flutter (Riverpod, `ory_client`), React + TanStack Query.

## 1. Traceability

| Req | Component(s) | Interface | Test |
| --- | --- | --- | --- |
| FR-01 resolve | `domain/login`, `app.LoginIdentifierService.Resolve`, `httpapi` `POST /v1/auth/identifiers` | I1 | unit (normalise, pseudonym, limiter), httpapi test, integration |
| FR-02 persist on registration | `LoginIdentifierService.Resolve`, `postgres.LoginIdentifierRepo.InsertIfAbsent` | I1 | integration (row present/absent) |
| FR-03 schema v2 | `deploy/ory/kratos/identity-schemas/customer.v2*.json`, kratos.yml | I11 | itest registration plaintext → 400 |
| FR-04 pre-persist check | `app.LoginIdentifierService.ValidateRegistration`, webhook handler | I8 | webhook unit + itest |
| FR-05/06 courier | `app.CourierDispatcher`, `domain/login/messages.go`, mailer, `adapter/sms` | I10 | unit (decision table, templates), itest via Mailpit |
| FR-07 /v1/me | `MeService.GetMe` + `LoginIdentifierService.Own` | I2 | httpapi + itest |
| FR-08/09 mobile | `LoginIdentifierResolver`, `AuthRepository`, 4 screens, settings re-auth | — | widget + integration |
| FR-10 admin masked | `CustomerService.List/Get` + `LoginIdentifierService.MaskMany` | I3/I4 | unit (1 batch call), itest |
| FR-11 lookup | `CustomerService.LookupByLogin` | I5 | unit + itest + web test |
| FR-12 reveal | `PersonalInfoService.Reveal` (+ `login` field) | I6 | unit + itest |
| FR-13 migration | `app.LoginMigrationService`, `kratos.LoginTraitAdmin` | I12 | itest (3 fixtures + crash) |
| FR-14 scrub | `deploy/ory/kratos/scrub/scrub-courier.sql`, `make kratos-scrub` | I13 | itest SQL probe |
| FR-15/16 purge/bind | `LoginIdentifierService.Bind/Purge` | I9, I12 | itest |
| FR-17 erasure | purge (orphan) + `reapply-erasures` | I12 | unit |
| FR-19 docs | docs/adr/0013, 08-pii, 03-auth-flows, 06-security, 01-overview, 04-data, 05-deployment, api docs | — | review |
| NFR-01 | itest `kratos_probe_test.go` (test-only SQL to kratos DB) | — | itest |
| NFR-02 | openbao init + policy | — | openbao integration test |
| NFR-03 | `login.AAD` | — | unit swap test (localkms + openbao itest) |
| NFR-04 | strict body middleware | — | router test |
| NFR-05 | `KeyedLimiter`, routes | — | unit |
| NFR-06 | error mapping | — | fault tests with fake key manager |
| NFR-07 | logger redaction keys | — | log capture test |
| NFR-08 | single code path | — | unit |
| NFR-10 | `CourierDispatchRepo` reservation | — | unit + repo itest |
| NFR-11 | admin pass-through | — | existing admin itests |

## 2. Components

### 2.1 `internal/domain/login` (new, pure)

| File | Content |
| --- | --- |
| `identifier.go` | `type Kind string` (`email`,`phone`); `type Identifier struct{Kind; Value string}`; `Parse(kind, raw string, p PhonePolicy) (Identifier, error)` returning `*FieldErrors`-compatible codes (`invalid_format`, `too_long`, `invalid_characters`, `unsupported_country`); email: trim, lower, `net/mail.ParseAddress` round-trip equality, ≤ 254, reject control/Cf chars (reuse `profile.HasControlChars` logic by copying the predicate into domain/login to keep domain packages independent); phone: strip `[ \-.()]`, `00`→`+`, leading `0` → `+<default cc>` (default `84`), regex `^\+[1-9][0-9]{7,14}$`, allowed country prefixes check (longest-prefix over the allow-list). |
| `pseudonym.go` | `const Domain = "login.invalid"`; `type Pseudonym [32]byte`; `(Pseudonym) String()` = lower base32 no padding + `@` + Domain; `ParsePseudonym(s) (Pseudonym, bool)` strict (`^[a-z2-7]{52}@login\.invalid$` case-insensitive input lower-cased); `PseudonymInput(id Identifier) []byte` = `"login-id/v1\x00"+kind+"\x00"+value`; `AAD(kind, p) []byte` = `"identity-service/login/v1\x00"+kind+"\x00"+hex(p)` |
| `mask.go` | `Mask(id Identifier) string` per contract §2 |
| `messages.go` | `Message{Subject, Text, SMS string}`; `Render(t TemplateType, locale string, code string, expires int) (Message, error)`; template types `verification_code_valid`, `recovery_code_valid`; locales `vi` (default), `en`; SMS ≤ 160 GSM-7 (ASCII-only Vietnamese) |
| `purpose.go` | `Purpose` enum + `Persists()` |

### 2.2 Ports (`internal/app/ports.go` additions)

```go
// LoginKeys computes login pseudonyms and seals contact values (DD-01, DD-03).
type LoginKeys interface {
    Pseudonym(ctx context.Context, input []byte) (login.Pseudonym, error)
    SealLogin(ctx context.Context, ad, plaintext []byte) (ct string, kekVersion int, err error)
    OpenLogins(ctx context.Context, items []SealedLogin) ([][]byte, error) // same order; per-item error → ErrDataIntegrity
}
type SealedLogin struct{ AD []byte; Ciphertext string }

type LoginRecord struct {
    Pseudonym login.Pseudonym; Kind login.Kind; Ciphertext string; KEKVersion int
    IdentityID *uuid.UUID; BoundAt *time.Time; LegacyVerified bool
    CreatedAt, LastValidatedAt time.Time
}
type LoginIdentifierRepo interface {
    InsertIfAbsent(ctx, r LoginRecord) (inserted bool, err error)
    Get(ctx, p login.Pseudonym) (LoginRecord, error)            // ErrNotFound
    GetMany(ctx, ps []login.Pseudonym) (map[login.Pseudonym]LoginRecord, error)
    GetByIdentity(ctx, id uuid.UUID) (LoginRecord, error)
    Bind(ctx, p login.Pseudonym, id uuid.UUID, replaceStale bool) (bool, error) // only when unbound, or bound to the same id, or replaceStale
    Touch(ctx, p login.Pseudonym) error                          // last_validated_at = now()
    SetLegacyVerified(ctx, p login.Pseudonym, v bool) error
    ListUnbound(ctx, before time.Time, limit int) ([]LoginRecord, error)
    ListBound(ctx, after login.Pseudonym, limit int) ([]LoginRecord, error)
    Delete(ctx, p login.Pseudonym, onlyIfUnbound bool) (bool, error)
    UpdateCiphertext(ctx, p login.Pseudonym, old string, ct string, v int) (bool, error)
    CountUnbound(ctx) (int64, error)
}
type CourierDispatchRepo interface {
    Reserve(ctx, key [32]byte, staleBefore time.Time) (reserved bool, err error) // false if sent or fresh pending
    MarkSent(ctx, key [32]byte) error
    Release(ctx, key [32]byte) error
    Purge(ctx, before time.Time) (int64, error)
}
type SMSSender interface { SendSMS(ctx context.Context, toE164, text string) error }
// Mailer gains: SendLoginMessage(ctx, to string, m login.Message) error
type LoginTraitAdmin interface {
    ListCustomers(ctx, pageToken string, size int) ([]identity.Identity, string, error)
    FindByIdentifier(ctx, identifier string) ([]identity.Identity, error)
    ReplaceLoginTrait(ctx, id uuid.UUID, oldEmail string, newLoginID string) error // ErrConflict / ErrNotFound
    MarkLoginVerified(ctx, id uuid.UUID, loginID string) error
}
```

`identity.Identity` and `identity.Principal` gain `LoginID string`,
`LoginVerified bool` (DD-16). The Kratos model reads `traits.login_id` and
falls back to `traits.email` (legacy); `Email`/`EmailVerified` keep being
filled from whichever is present **only for admins and legacy customers**
— for pseudonym customers `Email = ""`.

### 2.3 `app.LoginIdentifierService` (new)

Deps: `Keys LoginKeys`, `Repo LoginIdentifierRepo`, `Kratos LoginTraitAdmin`
(+ `IdentityAdmin`), `Audit/Tx`, `Clock`, `Phone PhonePolicy`,
`Phase MigrationPhase`, limiters (`ResolveIP`, `RegisterIP`, `InsertGlobal`),
`Log`, `Metrics` hooks.

| Method | Behaviour |
| --- | --- |
| `Resolve(ctx, ip netip.Addr, kind, raw, purpose)` | limiter by purpose (IP bucket) → `login.Parse` → `Keys.Pseudonym` → if `purpose.Persists()`: global insert limiter; `SealLogin(AAD, value)`; `InsertIfAbsent` (always seal, even if row exists, NFR-08). Returns pseudonym string |
| `ValidateRegistration(ctx, traits)` | A2 / I8 table; `Touch` on success |
| `Bind(ctx, id, loginID)` | parse pseudonym (legacy email → no-op); `Repo.Bind(…, replaceStale=false)`; if not bound because bound to another id → `IdentityAdmin.GetIdentity(other)`: 404 → `Bind(replaceStale=true)` (A6); else log `login_identifier_bound_elsewhere` |
| `Own(ctx, p Principal)` | legacy → `{email, p.LoginID}`; else `GetByIdentity` → if ErrNotFound: `Get(pseudonym)` + Bind (lazy, A1: principal comes from whoami, authoritative) → `OpenLogins` → `Identifier` |
| `MaskMany(ctx, idents)` | `GetMany` → one `OpenLogins` batch → `Mask`; errors → per-item `nil` + `unavailable=true` |
| `Reveal(ctx, id)` | as `Own` without lazy bind |
| `Lookup(ctx, kind, raw)` | parse → pseudonym → `FindByIdentifier(pseudonym)` (+ plaintext in transition) → customers only |
| `Purge(ctx, olderThan, dryRun)` | A10/A6: unbound rows `last_validated_at < now-olderThan` (SKIP LOCKED) → `FindByIdentifier(pseudonym)`: found customer → bind; none → delete (`onlyIfUnbound`). Bound rows: `GetIdentity` 404 → delete + audit `customer.login.erased` |
| `Rewrap(ctx)` | page `ListBound`+unbound, open + seal with latest version, `UpdateCiphertext` compare-and-swap |

### 2.4 `app.CourierDispatcher` (new)

Deps: `Logins *LoginIdentifierService`, `Identities IdentityAdmin`,
`Profiles ProfileRepo` (locale), `Mailer`, `SMS SMSSender`, `Dedupe
CourierDispatchRepo`, `DedupeKey []byte` (secret), `RecipientLimiter`
(per pseudonym 5/h, 20/day), `SMSBudget` (per country/day + global/day),
`Phase`, `Log`, `Metrics`.

`Dispatch(ctx, Msg) (Outcome, error)` — Outcome ∈ {`sent`, `duplicate`,
`dropped(reason)`}; error only for transient failures (→ 503). Order:

1. Validate (allow-listed template, code regex, identity id) → else drop `bad_payload`.
2. `key = HMAC(DedupeKey, type‖0‖recipient‖0‖code)`; `Reserve(key, now-2m)` → not reserved ⇒ `duplicate`.
3. Resolve target:
   - pseudonym → `Repo.Get` (none ⇒ drop `unresolved`); bound to `identity_id`? else if unbound: `GetIdentity(identity_id).LoginID == recipient` ⇒ Bind; else drop `unbindable`.
   - plaintext → `GetIdentity(identity_id)`: admin with `Email == recipient` ⇒ email; customer legacy with `LoginID == recipient` and `Phase == transition` ⇒ email; else drop `plaintext_refused`.
4. Quotas: recipient limiter; SMS country + global budget ⇒ drop `over_quota`.
5. Locale = profile locale prefix (`vi`/`en`, default `vi`); `login.Render`.
6. Decrypt (pseudonym) → send via Mailer / SMS. Send error ⇒ `Release(key)`, return transient error. Success ⇒ `MarkSent`.
Every drop: `Release` not needed (drops keep the reservation as `sent` to avoid retries re-processing → `MarkSent`), metric `courier_dropped_total{reason}`, log `courier_dropped reason=…` (no values).

### 2.5 `app.LoginMigrationService` (new, DD-12)

`Migrate(ctx, dryRun)`: page customers via `LoginTraitAdmin.ListCustomers`.
Per identity:
- `LoginID` is a pseudonym: if row `LegacyVerified` and identity not verified → `MarkLoginVerified` (counter `reverified`); else `skipped`.
- legacy email: parse (`kind=email`) → pseudonym → seal → `InsertIfAbsent` with `identity_id` bound and `legacy_verified = EmailVerified`; if a row exists bound to another **existing** identity → `failed` (log `login_migration_collision`, id only); then `ReplaceLoginTrait(id, email, pseudonym)` (conflict → `failed`, retry next run); then if verified `MarkLoginVerified`; audit `customer.login.migrated` (details `{kind}`).
- Invalid legacy email → `failed` (manual).
Prints counters; exit 1 if `failed > 0`.

### 2.6 Adapters

| Adapter | Change |
| --- | --- |
| `openbao` | `Pseudonym` → `transit/hmac/<login-hmac>/sha2-256` (`key_version: 1`, base64 input, parse `vault:v1:`); `SealLogin` → `transit/encrypt/<login-kek>` with `associated_data`; `OpenLogins` → `transit/decrypt` `batch_input` (chunks of 100), per-item errors → `ErrDataIntegrity`, HTTP 400 with batch results handled (S6). Config `PII_OPENBAO_LOGIN_HMAC_KEY_NAME` (default `identity-login-pseudonym`), `PII_OPENBAO_LOGIN_KEK_NAME` (default `identity-login-kek`); all four key names must be distinct |
| `localkms` | same port with local keys from `PII_LOCAL_LOGIN_HMAC_KEY`/`PII_LOCAL_LOGIN_KEK` (AES-GCM, AD), `local:v1:` prefix |
| `postgres` | migration `0006_login_identifier.sql` (tables per arch §4 + A10 + A9 states, grants: SELECT, INSERT, DELETE; column UPDATE on `identity_id, bound_at, legacy_verified, last_validated_at, value_ct, kek_version` for `login_identifier`; `courier_dispatch` SELECT/INSERT/UPDATE(state, updated_at)/DELETE); queries `db/queries/login.sql`; repos in `adapter/postgres/login.go` |
| `kratos` | model: `traits.login_id`; `LoginTraitAdmin` impl; `ListIdentities` keeps `credentials_identifier` |
| `mailer` | `SendLoginMessage` (text/plain, same compose/send) |
| `sms` (new pkg) | `Sink` (local: sends an email to `<digits>@sms.local` via `Mailer` with the SMS text — visible in Mailpit), `HTTP` (POST JSON `{to, text}` to `SMS_HTTP_URL` with `Authorization: Bearer SMS_HTTP_TOKEN`, https required unless local, 5 s timeout, non-2xx → transient) ; `SMS_PROVIDER` ∈ {`sink`,`http`,`disabled`}; `disabled` ⇒ resolve with `type=phone` → 422 `{field: type, code: unsupported}` |
| `httpapi` | public route `POST /v1/auth/identifiers` mounted **outside** the bearer-authenticated `/v1` group with its own middleware (IP limiter, strict body, no CORS credentials); `RoutePolicies` gains `Plane: PlanePublic` (new) validated to have no permission; webhook handler adds `pre-registration` (webhook key) and `courier` (courier key); `/admin/v1/customers` drops `email` param; lookup oneOf; reveal `login` |
| `platform/logger.go` | redaction keys (A12) |
| `platform/config.go` | new env (below) with validation |

### 2.7 Configuration (identity-service)

| Env | Default | Notes |
| --- | --- | --- |
| `KRATOS_COURIER_API_KEY` | — (required) | ≠ webhook key |
| `COURIER_DEDUPE_SECRET` | — (required, base64 ≥ 32 B) | HMAC key for dedupe |
| `LOGIN_MIGRATION_PHASE` | `transition` | `transition` \| `complete` |
| `LOGIN_PHONE_DEFAULT_COUNTRY` | `84` | |
| `LOGIN_PHONE_ALLOWED_COUNTRIES` | `84` | comma list |
| `LOGIN_RESOLVE_RATE` | `20/1m,200/24h` | parsed rules |
| `LOGIN_REGISTER_RATE` | `5/1m,30/24h` | |
| `LOGIN_INSERT_GLOBAL_RATE` | `120/1m` | |
| `COURIER_RECIPIENT_RATE` | `5/1h,20/24h` | |
| `SMS_PROVIDER` | `sink` local / `disabled` otherwise | |
| `SMS_DAILY_BUDGET` | `1000` | global; per-country = same unless `SMS_COUNTRY_BUDGETS` |
| `SMS_HTTP_URL`, `SMS_HTTP_TOKEN` | — | required when `http` |
| `PII_OPENBAO_LOGIN_HMAC_KEY_NAME`, `PII_OPENBAO_LOGIN_KEK_NAME` | see above | |

### 2.8 Kratos / infra

- `identity-schemas/customer.v2.transition.json`, `customer.v2.json`;
  `kratos.yml.tmpl`: schema URL → transition; `methods.profile.enabled:
  false`; registration hooks `[pre-registration web_hook (parse: true, can_interrupt), after-registration web_hook (ignore), session]`;
  `courier.delivery_strategy: http`, `courier.http.request_config` with
  `__KRATOS_COURIER_API_KEY__`; `courier.message_retries: 10`;
  drop `courier.smtp` (kept commented for reference? no — removed;
  `COURIER_SMTP_CONNECTION_URI` env removed from compose).
- webhooks: `pre-registration.jsonnet`, `courier.jsonnet`, `after-registration.jsonnet` (+`login_id`).
- compose: `kratos-config` renders the courier key; identity-service env adds
  the new secrets + `SMS_PROVIDER: sink`; `.env.example` adds
  `KRATOS_COURIER_API_KEY`, `COURIER_DEDUPE_SECRET`.
- OpenBao `init.sh`: create keys `identity-login-pseudonym` (`type=hmac`? —
  Transit HMAC works on any key; use `aes256-gcm96` keys with `hmac` endpoint
  as for `identity-pii-bidx`, auto-rotate off) and `identity-login-kek`
  (`aes256-gcm96`), `deletion_allowed=false`; policy
  `identity-service.hcl` adds the 3 paths; operator policy adds rotate/config
  for `identity-login-kek` (not for the HMAC key).
- `deploy/ory/kratos/scrub/scrub-courier.sql` + `make kratos-scrub`
  (`psql` in the postgres container as the kratos role): delete
  `courier_message_dispatches`/`courier_messages` older than `:keep` (default
  7 days) **and** any courier message whose `recipient` does not end with
  `@login.invalid` and belongs to a customer… (Kratos messages do not link
  to identities ⇒ rule: when `phase=complete`, delete all messages whose
  recipient is not a pseudonym and not an admin address; the script takes an
  admin-address allow-list from `identities` where `schema_id='admin'`).
- `Makefile`: `kratos-scrub`, `login-migrate`, `login-purge`.

### 2.9 Mobile (Flutter)

- `core/identity/login_identifier_client.dart`: `POST /v1/auth/identifiers`
  (no auth header), maps 422 codes to field errors, 429 → retry-after message, 503 → service unavailable.
- `features/auth/domain/login_input.dart`: `LoginType`, client-side format
  validation (UX only), normalisation preview not required.
- `AuthRepository`: `register/signIn/startRecovery/startVerification` take
  `LoginInput`, resolve first, call Kratos with the pseudonym. Successful
  sign-in stores `pseudonym` in secure storage keyed by
  `sha256(type:normalised-ish value)`; next sign-in re-uses it (A5).
- `kratos_models.dart`: `Identity.loginId` (`traits.login_id` ?? `traits.email`), `loginVerified`.
- Screens: sign up / sign in / forgot password: segmented control
  Email | Phone, field keyboard type, autofill hints; verify screen shows
  the contact from `/v1/me` and resends with the session's `loginId`.
- Settings re-auth: identifier = `session.identity.loginId` (FR-09).
- Profile: shows `me.login`. l10n vi/en strings; message ids 4049001/4049002 mapped.
- Password hint: "Don't use your email or phone in your password" (R-06).

### 2.10 Admin web

- `features/customers`: list column "Login" (masked + type icon), search box
  → `POST /customers/lookup {login}` (type auto-detected: contains `@` ⇒
  email else phone) replacing `?email=`; detail shows masked login; reveal
  dialog field `login`. `api/schema.d.ts` regenerated. i18n vi/en.

## 3. Cross-cutting

| Concern | Spec |
| --- | --- |
| Security | Sections A1–A14; no plaintext in logs; strict bodies; separate webhook keys; fail closed |
| Observability | Metrics: `login_resolve_total{purpose,result}`, `login_vault_inserts_total`, `login_unbound_rows` (gauge, purge job), `courier_dispatch_total{channel,result}`, `courier_dropped_total{reason}`, `sms_budget_remaining`; logs with request id; spans `login.resolve`, `courier.dispatch` |
| Performance | resolve: 1 Transit call (sign-in) / 2 (registration) + 1 DB insert; `/v1/me` +1 DB +1 Transit; admin list +1 DB +1 Transit batch |
| Data lifecycle | Unbound rows ≤ 24 h + purge interval; bound rows live with the identity; orphans deleted by purge (daily); courier_dispatch rows 24 h; Kratos courier messages ≤ 7 d (scrub); Kratos flows ≤ 24 h after expiry (`cleanup --keep-last 24h`) |
| Backups | identity DB ciphertext + OpenBao snapshots; KEK rotation per backup retention (same as PII) |

## 4. Work-breakdown seeds (dependency order)

1. U1 Domain `login` (+ tests).
2. U2 Ports + key adapters (openbao, localkms) + OpenBao init/policies.
3. U3 DB migration 0006 + sqlc queries + repos.
4. U4 `LoginIdentifierService` (resolve, validate, bind, own, mask, reveal, lookup, purge, rewrap).
5. U5 `CourierDispatcher` + mailer extension + SMS adapters.
6. U6 HTTP: OpenAPI changes + regen, public route, webhooks, admin changes, config, logger redaction, main wiring, CLI commands.
7. U7 Kratos config/schemas/jsonnet + compose + scrub script + Makefile.
8. U8 Migration service + CLI.
9. U9 Mobile.
10. U10 Admin web.
11. U11 Docs + ADR-0013.
12. U12 Integration/e2e tests (Launch).
