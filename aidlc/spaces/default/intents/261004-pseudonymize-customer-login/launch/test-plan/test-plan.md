# Test Plan — Pseudonymous customer login identifiers

## 1. Risk ranking (impact × likelihood)

| Rank | Risk | Requirements | Depth |
| --- | --- | --- | --- |
| 1 | A customer address is left in the Kratos DB, or reaches logs, errors or metrics | NFR-01, NFR-07, FR-14 | Unit (redaction, value-free errors), integration SQL probe, smoke SQL probe, log capture |
| 2 | Codes reach the wrong person (cross-identity delivery, open relay) or are lost (retry and quota interplay) | FR-05, NFR-10, A3/A4/A11 | Decision-table unit tests, webhook HTTP tests, e2e via Mailpit |
| 3 | Customers locked out: migration loses verification or crashes mid-way, pseudonym mismatch between replicas or keys | FR-13, A2, R-03 | Unit (crash/repair/collision), integration against real Kratos (two-patch), smoke after `./dev up` (migrate runs) |
| 4 | Registration bypass: plaintext or unresolved identifiers persisted | FR-03/04, A1/A2 | Real Kratos (spikes S1/S3/S7/S8 + smoke 4049001/4049002), webhook unit tests |
| 5 | Abuse: enumeration, SMS pumping, storage spam | NFR-05/08, A3/A5/A8 | Limiter unit tests (per-IP, /64, /24, registration, insert alert), courier quota tests (durable counts) |
| 6 | Key handling: AEAD binding, key separation, fail closed | NFR-02/03/06 | Adapter tests (OpenBao fake, localkms), swap tests, OpenBao integration (policy) |
| 7 | Client regressions (both login types, re-auth, PII in URLs/caches) | FR-08..12 | Widget/unit (Flutter), component (Vitest + MSW), Flutter integration against the stack |

## 2. Strategy

| Level | Where | Floor |
| --- | --- | --- |
| Unit (Go) | `domain/login`, `app` (login, courier, migration, reveal/lookup), adapters (openbao, localkms, kratos, sms, mailer), httpapi (public plane, strict body, hooks, Retry-After), platform (config) | Every FR/NFR with an AC has at least one named test (`TestPLI…`, `TestSECC…`) |
| Integration (Go, `-tags integration`) | Postgres repos (`login_integration_test.go`: bind/stale/dedupe/counts/conditional delete), Kratos adapter against the real Kratos, OpenBao Transit (`openbao_integration_test.go`), e2e in-process service against the stack (`internal/e2e`) | Run against this branch's stack |
| End-to-end | `scripts/smoke.mjs` (email + phone, 4049001/4049002, `/v1/me` login, SQL probe of Kratos tables, recovery, population guard), `scripts/seed-customers.mjs` (5 customers incl. 1 phone), Flutter `kratos_native_flow_test.dart` (`-P integration`) | All green on a freshly deployed stack |
| Client | Flutter `flutter test` (174), admin-web Vitest (130) | All green |

Additions in Test Generation (gaps found while planning):

1. **G1:** OpenBao integration — the app token can `hmac` with
   `identity-login-pseudonym` and `encrypt`/`decrypt` with
   `identity-login-kek`, but cannot read or rotate them (NFR-02).
2. **G2:** e2e — the courier webhook against real Kratos and Mailpit is
   covered by smoke/seed. An in-process Go e2e for login lookup + reveal of
   `login` + an audit row without the value (FR-11/12, NFR-07).
3. **G3:** migration integration — a legacy customer created through the
   Kratos admin API with the transition schema → `Migrate` → sign in with
   the pseudonym, still verified (FR-13, against real Kratos).
4. **G4:** log capture in e2e — no test address appears in the in-process
   service logs (NFR-07).

Deliberately not tested:

- The production SMS provider (port only; the HTTP adapter is covered by
  httptest).
- Load and performance beyond the timing notes (NFR-09 is a Should; recorded
  in release validation).
- Multi-replica limiter behaviour (documented per-replica semantics).

## 3. Environments and data

| Env | What | Data |
| --- | --- | --- |
| Unit | in-process fakes (`testutil`), `localkms`, httptest | Synthetic: `alice.login@example.com`, `0912 345 678`, `example.local` addresses. Phone numbers `+849…` are fictional |
| Integration/E2E | Docker Compose stack **built from this branch** (`make up` / `./dev up`): Postgres, Kratos v26.2.0 (transition schema, http courier), OpenBao 2.4.1 (+ new keys), Mailpit (email + SMS sink), identity-service | Unique random addresses per run (`smoke-<uuid>@example.local`, random `+849…`); identities deleted in test cleanup |
| Kratos config check | isolated `oryd/kratos:v26.2.0` with SQLite and the rendered template (already done in Develop) | — |

Constraint: a stack named `go-ory-auth-example` from **another checkout** is
already running on the same ports. Integration and e2e need this branch's
build, which means replacing that stack. **This needs the owner's
confirmation** (asked at Deployment). The fallback is a separate compose
project on shifted ports.

No production data, ever.

## 4. Exit criteria

1. Go: `go build`, `go vet` (with and without `-tags integration`), and
   `go test ./...` green. `go test -tags integration ./...` green against
   this branch's stack.
2. Mobile: `flutter analyze` clean; `flutter test` green; integration test
   green against the stack.
3. Admin web: typecheck, lint, test and build green.
4. Smoke: every check passes, including `PLI-NFR-01` (0 Kratos rows hold the
   test email or phone) and the phone/SMS path.
5. Migration: `./dev up` runs `migrate-logins` with `failed=0`; a second run
   reports `migrated=0`.
6. No test address in captured service logs (G4).

Known acceptable gaps: multi-replica limits, production SMS provider,
performance only measured informally.

## 5. Approval

Mode yolo: auto-approved. Execution against a stack needs the confirmation in
§3.
