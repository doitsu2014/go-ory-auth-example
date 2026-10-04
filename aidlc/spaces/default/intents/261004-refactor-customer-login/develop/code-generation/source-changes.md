# Source changes

## identity-service (Go)

| File | Change | Req |
| --- | --- | --- |
| `db/migrations/0007_login_lookup_key.sql` | new `lookup_key` (backfill = pseudonym, NOT NULL, len 32, UNIQUE) | PLX-FR-04 |
| `db/queries/login.sql`, `sqlcgen/*` | queries select `lookup_key`; `GetLoginIdentifierByLookupKey`; insert `ON CONFLICT DO NOTHING` (either key) | FR-04 |
| `domain/login/pseudonym.go` | `LookupKey`, `NewPseudonym()` (crypto/rand), `PseudonymInput` → `LookupInput` | FR-04 |
| `domain/login/purpose.go` | removed (resolve purposes) | FR-05 |
| `app/ports.go` | `LoginKeys.LookupKey`, `LoginRecord.LookupKey`, `LoginIdentifierRepo.GetByLookupKey` | |
| `app/login.go` | `Resolve` and resolve limiters removed; `parse`, `lookupKey`, `find`, `claim`, `store(k, p)`; admin `FindCustomers` through the vault | FR-02, 09 |
| `app/customerauth.go` (new) | `CustomerAuthService` (Login / Register / StartRecovery), `AuthFlows` port, `AuthFlowError`, `AuthSession` (redacting), decoy handle, legacy candidates, limits | FR-01..03, 07, NFR-02, 03 |
| `app/loginmigration.go` | migration claims a random handle (or reuses the row) | FR-04 |
| `adapter/kratos/selfservice.go` (new) | native API flows server to server; 400 flow → message ids only; 10 s client; `X-Forwarded-For` / `User-Agent` | FR-01..03, 06, NFR-04 |
| `adapter/openbao`, `adapter/localkms` | `Pseudonym` → `LookupKey` | |
| `adapter/postgres/login.go` | `GetByLookupKey`, `LookupKey` mapping | |
| `adapter/httpapi/*` | handlers `CustomerLogin` / `CustomerRegistration` / `CustomerRecovery`; policies; strict bodies; problem `auth_flow_rejected` (400); public client (IP + UA) in context | FR-05, 06, NFR-01 |
| `platform/config.go` | `LOGIN_SIGNIN_RATE`, `LOGIN_NET_RATE`, `LOGIN_ACCOUNT_RATE` (replace `LOGIN_RESOLVE_*`) | NFR-02 |
| `cmd/identity-service/main.go` | wires `CustomerAuthService` + `kratos.NewSelfService` | |
| `api/openapi/identity-service.v1.yaml`, `gen/api.gen.go` | 3 routes, 4 schemas, resolve removed | FR-05 |

## Mobile (Flutter)

| File | Change |
| --- | --- |
| `core/identity/customer_auth_client.dart` (new) | `CustomerAuthApi` / `HttpCustomerAuthApi`; `auth_flow_rejected` → synthetic `KratosFlow` (`login` / `password` / `form`) |
| `core/identity/login_identifier_client.dart`, `core/storage/login_identifier_cache.dart` | removed (resolver, device pseudonym cache) |
| `core/identity/login_input.dart` | `LoginPurpose`, `LoginTarget`, `PseudonymousLogin`, `normalised` removed |
| `core/kratos/*client.dart` | registration / recovery-start methods removed; `submitRecovery` takes the code only |
| `features/auth/data/auth_repository.dart` | login, register and recovery start through the proxy; verification with the session's handle |
| sign-in, sign-up, forgot password, verify screens | no device-side Kratos flow for sign-in and sign-up; field names `login` / `password`; an expired recovery flow returns to the email step |

## Other

- `apps/admin-web/src/api/schema.d.ts`: regenerated.
- `scripts/smoke.mjs`, `scripts/seed-customers.mjs`: use the proxy.
- `deploy/ory/kratos/{identity-schemas/customer.v2.json, webhooks/pre-registration.jsonnet}`: comments only.
- Tests: Go unit and integration tests, Flutter unit, widget and integration tests updated or added (see test-suite).
