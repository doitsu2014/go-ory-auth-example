# Code Generation Notes — Pseudonymous customer login identifiers

## Units and verification

| Unit | Status | Verification (exact commands, results) |
| --- | --- | --- |
| U1 domain `login` | done | `go test ./internal/domain/...` → ok |
| U2 key ports + OpenBao/localkms + init/policies | done | `go test ./internal/adapter/openbao/ ./internal/adapter/localkms/` → ok |
| U3 migration 0006 + sqlc + repos | done | `go tool sqlc generate`; `go vet -tags integration ./internal/adapter/postgres/` → ok (integration run in Launch) |
| U4 `LoginIdentifierService` | done | `go test ./internal/app/ -run PLI -v` → 12 tests PASS |
| U5 `CourierDispatcher` + mailer + SMS adapters | done | `go test ./internal/app/ ./internal/adapter/sms/` → ok |
| U6 OpenAPI + HTTP + config + wiring + CLI | done | `go tool oapi-codegen …`; `go test ./internal/adapter/httpapi/ ./internal/platform/` → ok |
| U7 Kratos schemas/config/jsonnet, compose, scrub, Makefile, dev, seed/smoke scripts | done | Real `kratos.yml.tmpl` rendered and served by `oryd/kratos:v26.2.0` in an isolated container: config valid; pre-registration, after-registration and courier payloads captured and match the Go structs field for field (strict decoding). `node --check scripts/*.mjs`, `bash -n dev` → ok |
| U8 migration service + Kratos `LoginTraitAdmin` | done | `go test ./internal/app/ ./internal/adapter/kratos/` → ok |
| U9 mobile | done | in `apps/mobile`: `flutter gen-l10n`, `dart format`, `flutter analyze` (no issues), `flutter test` (166 pass, 1 skipped = integration). Integration test compiles; it fails against the currently running stack because that stack is an older build from another checkout (no `/v1/auth/identifiers`) — re-run in Launch after deploying this branch |
| U10 admin web | done | in `apps/admin-web`: `pnpm generate:api`, `pnpm typecheck`, `pnpm lint`, `pnpm test --run` (14 files, 128 tests pass), `pnpm build` → all pass |
| U11 docs + ADR-0013 | done | review in Code Review |

Full Go suite: `cd services/identity-service && go build ./... && go vet ./... && go vet -tags integration ./... && go test ./...` → all packages ok.

## Spikes added during Develop

- **S7** (Kratos v26.2.0): with `methods.profile.enabled: false`, a settings submit with `method: profile` → 404 "endpoint disabled"; password settings still work.
- **S8**: transition schema (`login_id` xor legacy `email`, `minProperties/maxProperties: 1`) works. A legacy identity validates on admin PATCH (state). Pseudonym registration works. Sending both traits → 400 (4000001). The migration patch (remove email, add login_id) → 200.

## Deviations from the technical spec

| # | Spec | Implemented | Why |
| --- | --- | --- | --- |
| D1 | A10: purge with `FOR UPDATE SKIP LOCKED` row locks | Unlocked listing plus a **conditional delete** (`identity_id IS NULL AND last_validated_at < cutoff`); the pre-registration check fails if `Touch` finds the row gone | Holding row locks across Kratos admin calls would span an Ory call inside a DB transaction (forbidden by 02-backend-go). The conditional delete gives the same guarantee: a registration validated after the cutoff always wins |
| D2 | `localkms` login keys from new env vars `PII_LOCAL_LOGIN_*` | Derived from the configured local keys with HMAC and fixed labels | Local/test only; avoids two more secrets for developers while keeping key separation |
| D3 | api-contract: unknown body field on resolve → 400 `invalid_request` | 422 `validation_failed` with code `unknown_field` | Matches the existing strict-body convention (bodycheck.go) used by every other PII body |
| D4 | Audit action `customer.login.looked_up` | `customer.login.lookup` | Consistent with the existing `customer.pii.lookup` |
| D5 | Login reveal folded into the PII reveal response schema `PersonalInfo` | New response schema `RevealedPersonalInfo` | `PersonalInfo` is also the PUT request body (`additionalProperties: false`); adding `login` there would accept it on PUT |
| D6 | Phone lookups validated against the allow-list | Admin lookups accept any calling code (`PhonePolicy.AnyCountry`) | Admins must find numbers registered before an allow-list change; resolve/registration keep the allow-list |
| D7 | — | `GET /admin/v1/customers?email=` now returns 400 (strict params) | Deliberate: PII never in URLs (PLI-FR-11) |

## Notes

- `identity.Principal.EmailVerified` keeps its name and now means "login
  identifier verified" (DD-16). `Principal.Email` is empty for pseudonymous
  customers. `LoginID` is the Kratos identifier.
- Kratos `traits.email` JSON tag became `omitempty`, so customer patches never
  send an empty email.
- The webhook API key opens only `/after-registration`, `/after-login` and
  `/pre-registration`. `/courier` accepts only the courier key (tested both
  ways).
- Existing `deploy/compose/.env` files get the new keys appended by
  `./dev up` and `make env` (merge of missing keys from `.env.example`).

## Admin web deviations (U10)

| # | Spec | Implemented | Why |
| --- | --- | --- | --- |
| W1 | DD-19: TanStack mutation for the login lookup | Plain async call, results kept in component state | `useMutation` keeps `state.variables` (the typed email/phone) in the mutation cache, which breaks the existing rule that plaintext PII never enters TanStack caches. A test asserts both caches stay clean |
| W2 | Masked login inside the personal-info card | On the account card. The personal-info card shows the full login only while it is revealed | Stays visible even when the personal-info request fails |
| W3 | — | Client checks only that the lookup value is non-empty; the server normalises and validates (422 → localised message) | Normalisation rules live in one place (domain/login) |
| W4 | — | i18n `{{email}}` → `{{customer}}`; email-only keys removed | Customers no longer have a visible email |

## Mobile deviations (U9)

| # | Item | Decision |
| --- | --- | --- |
| M1 | New direct dependency `crypto: ^3.0.7` (sha256 cache key) | Already transitive at the same version. **Dependency change, needs review** (org rule) |
| M2 | `core/identity` imports `features/auth/domain/login_input.dart` | Kept the spec's paths; a core→feature import. Review may move `login_input.dart` into core |
| M3 | Verification takes a typed `LoginInput` (resolved) or a `PseudonymousLogin` (used as is) | The verify screen always uses the session pseudonym |
| M4 | Pseudonym cache | Cached after a successful sign-in only. Evicted on a Kratos 400 without an automatic retry. Storage errors are never fatal. Not cleared on logout |
| M5 | Client format checks run in the screens; the repository does none | The server is authoritative |
| M6 | Widget keys renamed to `<prefix>.login` / `<prefix>.loginType.*`; some strings now say "email or phone" | UX |

## Additional deviations from code review

| # | Item | Decision |
| --- | --- | --- |
| D8 | Courier template allow-list | Only `verification_code_valid` and `recovery_code_valid` are accepted. The contract also listed the `*_invalid` variants, which Kratos never sends while `notify_unknown_recipients` is false |
| D9 | A8 global insert cap | Alert only, never a refusal (SEC-C03). Abuse is bounded per IP and per /24 or /48 network |
| D10 | A3 quotas | Counted in the database (`courier_dispatch`), so they survive restarts and are shared by replicas. Concurrent sends can overshoot by the number of messages in flight |
