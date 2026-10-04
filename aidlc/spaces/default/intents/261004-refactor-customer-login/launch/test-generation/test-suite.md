# Test suite

## New or rewritten

- `services/identity-service/internal/app/login_test.go`: the proxy use cases
  (registration, login, legacy transition, recovery with a sealed ref, the
  stray unbound row, validation, rate limits including concurrency, fail
  closed) replace the resolve tests.
- `internal/adapter/kratos/selfservice_test.go`: the adapter against a fake
  Kratos (create and submit, forwarded headers, value-free rejection,
  verification flow id, failures).
- `internal/adapter/httpapi/login_test.go`: the HTTP routes, the problem
  shape, strict bodies, 404 on the old endpoint, recovery and the code
  endpoint (200 / 410).
- `internal/adapter/postgres/login_integration_test.go`: the lookup key.
- `internal/testutil/login.go`: fake `AuthFlows`; the fake repo enforces a
  unique lookup key.
- `internal/testutil/itest`: `RegisterCustomer` and `LoginCustomerAPI` go
  through the proxy; `Handle()` comes from whoami.
- `apps/mobile/test/core/customer_auth_client_test.dart` (new),
  `auth_repository_test.dart`, the screen tests and the integration test.
- `scripts/smoke.mjs`: 12 PLX checks.

## Removed

- The resolver client test and the pseudonym-cache tests (the feature is
  gone).
- The Ory client registration-body and recovery-body tests (the methods are
  gone).

## Results

Go unit tests pass; Go integration tests pass; smoke passes; Flutter has
163 unit tests passing and 7 integration tests passing.
