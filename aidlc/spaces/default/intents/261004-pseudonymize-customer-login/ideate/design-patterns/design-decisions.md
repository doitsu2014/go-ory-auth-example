# Design Decisions — Pseudonymous customer login identifiers

Patterns already in the codebase are preferred (02-backend-go): hexagonal
ports/adapters, sqlc repos, `audited()` mutation helper, in-memory
`RateLimiter`, read-compare-write migrations (`namemigration.go`), RFC 9457
error mapping, strict OpenAPI server. Deviations are marked **Δ**.

| # | Problem / forces | Decision | Alternatives | Consequences | Status |
| --- | --- | --- | --- | --- | --- |
| DD-01 | Pseudonym derivation must be deterministic, keyed, typed, server-only | **Keyed blind index** via a new `KeyManager`-style port `LoginKeys.Pseudonym(ctx, input []byte) ([32]byte, error)` (Transit HMAC, pinned v1); input built by `domain/login.PseudonymInput(kind, normalised)` | Local HMAC with env key (localkms only); SHA-256 unkeyed (dictionary-reversible) | Same pattern as `pii.PhoneBlindIndexInput`; localkms adapter implements it for tests/local-without-bao | Accepted |
| DD-02 | Value-object for typed identifier | `login.Identifier{Kind, Value}` constructed only by `login.Parse(kind, raw, opts)` (normalise + validate); `login.Pseudonym` type with `String()`/`ParsePseudonym()` (base32 + `@login.invalid`) | Strings everywhere | Invalid states unrepresentable; masking lives on the value object | Accepted |
| DD-03 | Encrypt small values, batch decrypt for lists | **Direct AEAD via key manager** `LoginKeys.Seal/OpenBatch` with per-item AD `login.AAD(kind, pseudonym)` | Envelope DEK (ADR-0011 pattern) | **Δ** from ADR-0011's envelope pattern: justified in architecture §1 (pre-identity storage, batch lists). No DEK cache needed | Accepted |
| DD-04 | Vault persistence | **Repository** `LoginIdentifierRepo` (sqlc): `InsertIfAbsent`, `Get(pseudonym)`, `GetMany`, `Bind(pseudonym, id, allowRebind func)`, `Touch(validated)`, `ListUnbound(before, limit) FOR UPDATE SKIP LOCKED`, `ListBound(after, limit)`, `Delete`, `ListForRewrap` | Store in `customer_pii` | Separate table = separate lifecycle (login survives PII erase) | Accepted |
| DD-05 | Courier delivery has to be idempotent under Kratos retries without a message id | **Idempotent receiver** with reservation (`pending → sent`) keyed by keyed-HMAC dedupe key (A9) | Outbox; no dedupe | At-least-once with a narrow duplicate window (crash after send, before mark) | Accepted |
| DD-06 | Channel selection email vs SMS | **Strategy** behind ports `Mailer.SendLoginCode` (existing SMTP adapter extended) and new `SMSSender.Send`; dispatcher picks by `login.Kind` | One generic "Notifier" | Keeps the existing Mailer port; SMS adapters: `smssink` (Mailpit via SMTP, local) and `smshttp` (generic JSON provider) chosen by `SMS_PROVIDER` | Accepted |
| DD-07 | Message content | **Template method** in `domain/login/messages.go`: `Render(templateType, locale, code, expires) (subject, text)` for vi/en; SMS text ≤ 160 GSM-7 chars (no diacritics in SMS vi variant) | Use Kratos templates | Kratos bodies never forwarded (A4); deterministic snapshot tests | Accepted |
| DD-08 | Abuse control on public resolve and on dispatch | Reuse **`RateLimiter`** (sliding log); **Δ** generalise its key from `uuid.UUID` to a comparable `string` key type via a small generic `KeyedLimiter[K comparable]`, keeping `RateLimiter` as an alias for `KeyedLimiter[uuid.UUID]` so callers don't change | New dependency (redis limiter) | Per-replica limits (×replicas in prod) — documented like existing limits | Accepted |
| DD-09 | Client IP | Reuse existing trusted-hop extraction (`TRUSTED_PROXY_HOPS`) + new `ipBucket(addr)` (/64 for IPv6, /32 IPv4) | X-Forwarded-For raw | — | Accepted |
| DD-10 | Webhook trust | **Separate authenticator per hook family** (`webhookAPIKey`, `courierAPIKey`), constant-time compare (existing pattern) | One shared key | Two secrets to provision in compose init container | Accepted |
| DD-11 | Admin pass-through and pseudonym binding need authoritative identity data | **Query the source of truth** (Kratos admin `GetIdentity`) inside the dispatcher before sending; never trust payload fields beyond ids/codes | Trust payload | +1 Kratos admin call per message (cheap, private network) | Accepted |
| DD-12 | Legacy migration of identities | **Read-compare-write migrator** modelled on `NameMigrationService`: page through Kratos admin, per-identity steps idempotent, crash-safe ordering (vault row before trait, trait before re-verify), counters, `--dry-run` | Big-bang SQL | Same operability and test approach as name migration | Accepted |
| DD-13 | Kratos rewrite operations | Extend Kratos adapter with a narrow port `LoginTraitAdmin`: `ReplaceLoginTrait(id, oldEmail, newLoginID)` (re-read, compare, JSON Patch remove `/traits/email` + add `/traits/login_id`), `MarkLoginVerified(id, loginID)` (patch `/verifiable_addresses/{i}`), `FindByIdentifier(identifier)` | Generic patch API | Ports stay intention-revealing; S4b two-patch encoded in one place | Accepted |
| DD-14 | Read model for `/v1/me` / admin views | **Join at use-case level**: identities from Kratos, contact from vault via `LoginIdentifierService.Reveal/MaskMany`; no caching of plaintext | Cache decrypted contacts | Extra Transit round trip per `/v1/me` (budget 80 ms p95) | Accepted |
| DD-15 | Phase flag for migration window | Config `LOGIN_MIGRATION_PHASE` ∈ {`transition`, `complete`} (default `transition` until an operator flips it); read once at start | DB flag | Restart to flip; explicit in runbook | Accepted |
| DD-16 | Principal shape | `identity.Principal` gains `LoginID string` (pseudonym or legacy email) and `LoginVerified bool`; `Email`/`EmailVerified` kept as derived aliases for admins only | Rename everywhere | Minimal churn; customers' `Email` empty after migration | Accepted |
| DD-17 | Erasure ledger | Purge job handles orphan rows; `pii reapply-erasures` also deletes `login_identifier` rows for identities with `customer.login.erased` events | Separate ledger | Re-uses existing ledger mechanism | Accepted |
| DD-18 | Mobile flow orchestration | **Repository + resolver** in Flutter: `LoginIdentifierResolver` (identity-service client, no auth) used by `AuthRepository` before Kratos calls; pseudonym cached in secure storage keyed by `sha256(type:value)` after successful sign-in | Resolve in widgets | Screens stay dumb; testable with fakes | Accepted |
| DD-19 | Admin web | TanStack Query mutation for lookup (POST), masked contact component reused from PersonalInfoCard style; revealed `login` cleared on unmount (existing rule) | — | — | Accepted |

## Deviations summary

- **DD-03** departs from ADR-0011's per-customer envelope for this one table
  (justified: pre-identity storage, batch reads, lifecycle independent of PII
  erase). Recorded in ADR-0013.
- **DD-08** generalises the limiter key type; existing call sites unchanged.
