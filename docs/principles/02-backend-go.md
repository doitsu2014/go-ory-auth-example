# Go Backend Guidelines (identity-service)

## 1. Stack

| Concern | Choice | Why |
| --- | --- | --- |
| Go | latest stable (go.mod `go` directive pinned) | — |
| HTTP router | `go-chi/chi/v5` | stdlib-compatible `http.Handler`, middleware ecosystem |
| API layer | `oapi-codegen` (strict server + models) from `api/openapi/identity-service.v1.yaml` | contract-first (P7) |
| DB driver / pool | `jackc/pgx/v5` + `pgxpool` | fastest native Postgres driver |
| Queries | `sqlc` (pgx/v5 output) | typed, parameterised SQL; no ORM magic |
| Migrations | `pressly/goose/v3` (SQL files, embedded) | simple, forward-only |
| Ory integration | thin hand-written HTTP adapters behind ports (`adapter/kratos`, `adapter/keto`) | `kratos-client-go` lags Kratos v26 and `keto-client-go` is unmaintained; only ~10 endpoints are needed. `ory/client-go` can replace an adapter later without touching `app` |
| Config | `caarlos0/env/v11` | typed env parsing |
| Logging | `log/slog` (JSON handler) | stdlib |
| Telemetry | OpenTelemetry Go SDK, `otelhttp`, `otelpgx` | vendor-neutral |
| Validation | generated OpenAPI validation + domain constructors | one place for rules |
| CLI | `spf13/cobra` | `serve`, `migrate`, `admin bootstrap` subcommands |
| Tests | `testing`, `stretchr/testify`, `testcontainers-go` | — |
| Lint | `golangci-lint` (govet, staticcheck, errcheck, gosec, revive, gofumpt) | — |

Dependency additions are reviewed (org rule) and recorded in the PR.

## 2. Layout

```
services/identity-service/
├── cmd/identity-service/        # main.go: cobra root → serve | migrate | admin
├── internal/
│   ├── domain/                  # pure business types & rules (no imports of http/sql/ory)
│   │   ├── identity/            # Principal, Kind(customer|admin), AAL
│   │   ├── profile/             # Profile entity, validation, errors
│   │   └── audit/               # Event, Action constants
│   ├── app/                     # use cases (application services) + ports
│   │   ├── ports.go             # interfaces: ProfileRepo, AuditRepo, IdentityAdmin, Authorizer, SessionVerifier, Clock
│   │   ├── me.go                # GetMe, UpdateMe
│   │   ├── customers.go         # ListCustomers, GetCustomer, DisableCustomer, EnableCustomer, RevokeSessions
│   │   ├── admins.go            # InviteAdmin, ChangeRole, ListAdmins
│   │   └── provisioning.go      # HandleRegistration (webhook), EnsureProfile (lazy)
│   ├── adapter/
│   │   ├── httpapi/             # generated server iface impl, middleware, problem+json mapping
│   │   ├── kratos/              # SessionVerifier (+cache) & IdentityAdmin implementations
│   │   ├── keto/                # Authorizer implementation
│   │   └── postgres/            # sqlc queries + repo implementations
│   ├── platform/                # config, logger, otel, db pool, http server bootstrap
│   └── testutil/                # containers, fixtures, fakes
├── db/
│   ├── migrations/              # goose: 0001_init.sql …
│   └── queries/                 # sqlc *.sql
├── sqlc.yaml
├── oapi-codegen.yaml
├── Dockerfile                   # multi-stage, distroless/static, non-root
└── Makefile
```

Dependency rule: `domain` ← `app` ← `adapter` ← `cmd/platform`. Arrows point
inward only; enforced with `depguard` in golangci-lint.

## 3. Request pipeline (middleware order)

1. `Recoverer` → 500 problem, logs stack with request id.
2. `RequestID` (accept inbound `X-Request-Id` if valid, else generate).
3. OTel `otelhttp` span.
4. Access log (slog) + metrics.
5. Body size limit (1 MiB), timeout (10 s).
6. CORS (admin origin allowlist, credentials).
7. `Authenticate`: the credential type is bound to the router. `/v1` reads only
   `Authorization: Bearer`. `/admin/v1` reads only the `ory_kratos_session` cookie
   and rejects `Authorization`/`X-Session-Token`. Then
   `SessionVerifier.Verify(ctx, cred)` → `Principal`, stored in context.
   Missing/wrong type → 401.
8. `RequirePlane`: `/v1` ⇒ `Kind == customer`. `/admin/v1` ⇒ `Kind == admin` +
   AAL2 + age ≤ 12 h. Past the cap, the session is revoked through Kratos admin
   `DELETE /admin/sessions/{id}`.
9. Per-route `Authorize(permission)` → Keto check.
10. CSRF guard for cookie-authenticated mutations (Origin + JSON content type).

`Principal` (domain type) is the only thing handlers see — never the raw Kratos session:

```go
type Principal struct {
    IdentityID      uuid.UUID
    Kind            Kind      // KindCustomer | KindAdmin
    AAL             AAL       // AAL1 | AAL2
    AuthenticatedAt time.Time
    ExpiresAt       time.Time
    EmailVerified   bool
}
```

## 4. Session verification & cache

- `kratos.SessionVerifier` calls `FrontendApi.ToSession` with `X-Session-Token`
  or the forwarded `Cookie` header.
- Cache: in-process LRU (e.g. `hashicorp/golang-lru/v2/expirable`), key =
  SHA-256 of the credential, TTL = `min(30s, expires_at-now)`; only positive
  results cached; 401 from Kratos is not cached.
- `Invalidate(identityID)` used by admin disable/revoke.
- Kratos `403 session_aal2_required` → `ErrAAL2Required` → HTTP 403 `aal2_required`
  (never 401, which would loop the admin on the login page).
- Kratos 5xx/timeout → `ErrDependencyUnavailable` → HTTP 503 (fail closed, P10).

## 5. Coding rules

- `context.Context` first parameter on every I/O function; respect cancellation.
- Errors: wrap with `fmt.Errorf("op: %w", err)`; domain sentinel errors
  (`profile.ErrNotFound`, `app.ErrForbidden`) mapped to problem codes in one
  place (`httpapi/errors.go`). Never leak internal error text to clients.
- No global state except the `main` wiring; constructors take dependencies
  explicitly (manual DI, no reflection containers).
- Transactions are owned by use cases via a `TxRunner` port; keep them short,
  never span an Ory call.
- Ory calls happen **before** or **after** DB transactions, and use cases are
  written to be re-runnable (e.g. invite: create identity → write tuple →
  audit; each step idempotent).
- Time via an injected `Clock`; IDs from Kratos or DB, never `rand` in domain.
- Logging: `slog.With("request_id", …)`; never log tokens, cookies, emails,
  passwords, codes. A redaction handler wraps the slog handler.
- Generated code (`*.gen.go`, sqlc output) is committed and CI checks it is
  up to date (`make generate && git diff --exit-code`).
- Public functions in `app` have doc comments describing the permission they require.

## 6. Webhook endpoints (port :8081, webhooks only; health/metrics on :9090)

- `POST /internal/hooks/kratos/after-registration` — body from Jsonnet template:
  `{ "identity_id": ctx.identity.id, "schema_id": ctx.identity.schema_id, "flow_type": ctx.flow.type }`.
- Auth: `Authorization: <api key>` compared in constant time.
- Response `204` quickly; work is a single idempotent upsert.

## 7. Testing

- Use cases: table-driven unit tests with fake ports (no containers).
- Adapters: integration tests with `testcontainers-go` Postgres (migrations
  applied in setup) and a Kratos/Keto container using `deploy/ory` config.
- HTTP: `httptest` against the real router with fake verifier to assert 401/403 matrices for every route.
- `go test -race ./...` in CI.

## 8. Commands

```bash
make generate          # oapi-codegen + sqlc
make lint              # golangci-lint run
make test              # unit tests
make test-integration  # testcontainers
go run ./cmd/identity-service migrate up
go run ./cmd/identity-service serve
go run ./cmd/identity-service admin bootstrap --email admin@example.local
```
