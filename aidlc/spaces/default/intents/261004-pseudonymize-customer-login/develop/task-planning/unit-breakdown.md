# Unit Breakdown — Pseudonymous customer login identifiers

Walking skeleton = **U1 → U2 → U3 → U4 (resolve + validate + bind) → U5
(dispatch email) → U6 (routes) → U7 (Kratos/compose)**: one customer registers
with an email pseudonym, gets the verification code in Mailpit at the real
address, verifies, signs in. Everything else hangs off that slice.

| Unit | Title | Reqs | Acceptance | Files (likely) | Deps | Size |
| --- | --- | --- | --- | --- | --- | --- |
| U1 | Domain `login` | FR-01, 06, 10; NFR-03, 08 | Parse/normalise table tests (email, VN phone forms, allow-list), pseudonym round-trip + strict parse, AAD distinct per kind/pseudonym, masking fixtures, templates vi/en snapshot, SMS ≤160 ASCII | `internal/domain/login/*.go` (+ `_test.go`) | — | M |
| U2 | Key ports + adapters + OpenBao | FR-01/02; NFR-02/03 | `LoginKeys` on openbao (hmac pinned v1, encrypt/decrypt batch with AD, 400 batch handling) and localkms; unit tests with httptest; init.sh + policies create/allow exactly the new paths; config validation (distinct key names) | `app/ports.go`, `adapter/openbao/openbao.go`, `adapter/localkms/localkms.go`, `deploy/openbao/init.sh`, `deploy/openbao/policies/*.hcl`, `platform/config.go` | U1 | M |
| U3 | Migration 0006 + repos | FR-02, 15, 16; NFR-10 | goose up/down; column grants; sqlc queries; repo integration tests (insert-if-absent, bind rules incl. stale, SKIP LOCKED listing, dedupe reserve/stale retake) | `db/migrations/0006_login_identifier.sql`, `db/queries/login.sql`, `adapter/postgres/login.go`, sqlcgen | U1 | M |
| U4 | `LoginIdentifierService` | FR-01, 02, 04, 07, 10–12, 15–17; NFR-05, 06, 08 | Unit tests with fakes: limiter per purpose/IP bucket, registration always seals, validate table (A2), bind/stale rebind, own/lazy bind, mask batch = 1 call, lookup both forms in transition, purge rules, rewrap CAS | `app/login.go`, `app/ratelimit.go` (KeyedLimiter), `app/login_test.go` | U2, U3 | L |
| U5 | `CourierDispatcher` + channels | FR-05, 06; NFR-10; A3, A4, A9, A11 | Decision-table unit tests (every drop reason, transient vs permanent, dedupe, quotas, admin pass-through only after Kratos confirm, legacy only in transition), Mailer `SendLoginMessage`, SMS sink + http adapters with httptest | `app/courier.go`, `adapter/mailer/smtp.go`, `adapter/sms/*.go` | U4 | L |
| U6 | HTTP + contract + wiring | FR-01, 04, 05, 07, 10–12; NFR-04, 05, 07 | OpenAPI updated + regenerated; public route outside bearer group, `PlanePublic` policy validated; webhooks pre-registration (Kratos error format) + courier (separate key) + after-registration bind; admin list without `email`, lookup oneOf, reveal `login`; logger redaction; `main.go` wiring + CLI `pii migrate-kratos-logins`/`purge-unbound-logins`; router/handler tests | `api/openapi/identity-service.v1.yaml`, `adapter/httpapi/*`, `platform/logger.go`, `cmd/identity-service/main.go` | U4, U5 | L |
| U7 | Kratos + compose + scrub | FR-03, 04, 05, 14 | Schemas transition/final validate; kratos.yml.tmpl (profile off, hooks, http courier, retries); jsonnet files; compose renders courier key, new env; `.env.example`; scrub SQL + Makefile targets | `deploy/ory/kratos/**`, `deploy/compose/*`, `Makefile` | U6 | M |
| U8 | Login migration service | FR-13 | Unit tests with fake Kratos (verified/unverified/crash-between-patches/collision/invalid); Kratos adapter `LoginTraitAdmin` httptest (patch bodies, read-compare) | `app/loginmigration.go`, `adapter/kratos/login.go`, `adapter/kratos/model.go` | U4 | M |
| U9 | Mobile | FR-08, 09 | Resolver client; AuthRepository resolves first; 4 screens with Email/Phone; `/v1/me` login display; settings re-auth uses `loginId`; l10n; widget tests green; integration test updated | `apps/mobile/lib/**`, `apps/mobile/test/**` | U6 | L |
| U10 | Admin web | FR-10–12 | Login column + masked detail + lookup by login + reveal field; MSW tests; schema types regenerated | `apps/admin-web/src/features/customers/**`, `api/schema.d.ts`, i18n | U6 | M |
| U11 | Docs + ADR | FR-19 | ADR-0013; 08-pii §8.1/§8.10/new §8.12; 03-auth-flows diagrams; 06-security risks; 01-overview non-goal; 04-data tables; 05-deployment runbook; api doc | `docs/**` | U6–U8 | M |
| U12 | Integration/e2e | NFR-01, 06, 07, SC 1–8 | itests against the stack: register/verify/sign-in email & phone, Kratos SQL probe, log capture, migration fixture, admin flows | `internal/testutil/itest`, `*_integration_test.go`, `scripts/smoke` | U7–U8 | L (Launch) |

## Risk ordering

Highest risk first inside the dependency order: U2 (Transit batch/AD
behaviour), U5 (courier semantics), U7 (Kratos config — validated by
spikes, but the compose wiring is new), U8 (two-patch migration).
