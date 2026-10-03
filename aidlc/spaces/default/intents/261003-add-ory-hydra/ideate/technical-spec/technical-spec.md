# Technical Specification — M2M with Hydra

## 1. Compose / config

- `deploy/ory/hydra/hydra.yml`:
  - `serve.public.port 4444`, `serve.admin.port 4445`;
  - `urls.self.issuer http://localhost:4444`;
  - `strategies.access_token: jwt`, `strategies.jwt.scope_claim: list`;
  - `ttl.access_token: 5m`;
  - `oauth2.client_credentials.default_grant_allowed_scope: false`;
  - `log.leak_sensitive_values: false`.

  Login and consent URLs point to an explicit "not supported" placeholder.
  Secrets come from env as `SECRETS_SYSTEM`.
- `deploy/postgres/provision/hydra.sh`: idempotent role + DB creation. It runs
  as the one-shot `hydra-db` job with the postgres superuser. The password is
  passed as a psql variable, as in 01-databases.sh.
- `.env.example`: `HYDRA_DB_PASSWORD`, `HYDRA_SECRETS_SYSTEM` (local values only).
- Services `hydra-db`, `hydra-migrate` (`migrate sql up -e --yes`) and `hydra`
  use image `oryd/hydra:v26.2.0`, with a healthcheck on `/health/ready`. Host
  ports are `4444` and `127.0.0.1:4445`.
- Makefile: `health` gains Hydra. `m2m-token` is a demo that prints a token
  for a given client, reading the secret from an env var.

## 2. Go (services/identity-service)

| Package | Contents |
| --- | --- |
| `domain/machine` | `Principal{ClientID string; Scopes []Scope}`, `Scope` enum (`customers:read`, `audit:read`), `ServiceClient`, validation (name pattern, owner email, scopes subset, non-empty) |
| `app` ports | `MachineTokenVerifier.Verify(ctx, token) (machine.Principal, error)` (errors `ErrInvalidToken`, `ErrDependencyUnavailable`); `ServiceClientAdmin{Create, Get, List, SetSecret, Delete}` (Hydra admin) |
| `app/serviceclients.go` | `ServiceClientService` (create/list/get/rotate/delete, audited, idempotent create with compensation) |
| `app/machine.go` | `MachineService` (customer status by id via Kratos admin, audit list reuse) |
| `adapter/hydra` | `admin.go` (client CRUD, `metadata.managed_by` filter, no-redirect client, value-free errors); `verifier.go` (go-jose v4: JWKS cache + refresh-on-unknown-kid ≤ 1/10 s, alg allowlist, claims checks with 30 s leeway, client-status cache 30 s LRU with purge) |
| `adapter/httpapi` | `PlaneMachine` and a `Scope` field on `Policy`, with `ValidatePolicies` requiring a scope for `/m2m/v1/*`; machine auth middleware (Bearer → verifier → scope → per-client limiter 600/min); `WWW-Authenticate` on 401/403; handlers for 7 operations |
| `cmd` | `clients create --name --owner --scope …` (system actor `00000000-0000-0000-0000-000000000000`, prints the secret once) |
| `platform/config` | `HYDRA_ADMIN_URL`, `M2M_JWKS_URL`, `M2M_ISSUER`, `M2M_AUDIENCE` (default `identity-service`), `M2M_CLIENT_CACHE_TTL` (30s), `M2M_RATE_LIMIT_PER_MIN` (600). https required for the JWKS and admin URLs unless `APP_ENV` is local or test |

New module: `github.com/go-jose/go-jose/v4`.

## 3. Keto

`manage_service_clients: (ctx) => this.related.super_admins.includes(ctx.subject)`.

## 4. Clients

The admin web gets a "Service clients" page for `manage_service_clients`:
- list;
- create dialog, where the secret is shown once in a dialog with a copy
  button and a "won't be shown again" warning, and is never put in the query
  cache;
- rotate (confirm, then the same one-time dialog);
- delete (confirm);
- vi/en strings and MSW tests.

Mobile: no changes.

## 5. Tests

| Level | What |
| --- | --- |
| Unit | Verifier with a local RSA key and httptest JWKS: valid; wrong alg (HS256 / none); missing or unknown kid with refresh rate-limited; wrong iss; aud missing/empty; expired/nbf with leeway; missing scp; client deleted (cache) → 401; Hydra down → 503 / cached ok. Policy validation fails for an `/m2m` route without a scope. Domain validation |
| Integration | Real Hydra: create client via the service → get a token from Hydra → call `/m2m/v1/customers/{id}` 200; wrong scope 403; Kratos token on m2m 401; Hydra JWT on `/v1/me` 401; rotate → old secret 401 at the token endpoint; delete → 401 after the cache purge; token without audience → 401 |
| Smoke | +5 checks: create client (CLI or Hydra admin), token, m2m call, insufficient scope, plane binding |

## 6. Security design review — resolutions (security-agent, 2026-10-03)

These override anything above that disagrees.

| # | Finding | Resolution |
| --- | --- | --- |
| B1 | The machine audit feed would leak `reason`, `ticket_ref`, `bidx`, `matched_ids` and PII field names | New `MachineAuditEvent` DTO with an **action allowlist**: customer.disabled/enabled/sessions_revoked, admin.*, service_client.*. All `customer.pii.*` actions are excluded. A **detail-key allowlist** (`role`, `previous_role`, `scopes`, `name`) applies; every other key is dropped. M2M-FR-06 = "admin audit list filtered through the allowlist". Contract: `/m2m/v1/audit-events` returns `MachineAuditEventPage` |
| B2 | Tokens from other Hydra clients or keys would be accepted | Require `sub == client_id`, a non-empty `scp` array, `aud` containing the audience, and **no** `nonce`/`at_hash`/`azp`. The client-status lookup also requires `metadata.managed_by == "identity-service"`, `grant_types == ["client_credentials"]` and `audience == ["identity-service"]`. Anything else → 401 |
| B3 | Rotating a secret left stolen tokens alive | Rotate writes `metadata.tokens_valid_after = now (unix)`, which is cached with the client status. Tokens whose `iat` is earlier than that are rejected with 401 |
| B4 | The Hydra admin API is reachable from every container | Compose networks: `hydra` (shared by Hydra and identity-service only) and `hydra-db` (Hydra, hydra-migrate, hydra-db and postgres). Hydra is **not** on `default`. Host ports: `4444` and `127.0.0.1:4445`. Production needs a NetworkPolicy. 06-security gets a T9 row and a Hydra hardening checklist |
| B5 | DB provisioning | Already implemented with `\gexec` + `ALTER ROLE` (verified idempotent; password passed via env/psql variable, nothing in logs; role is NOSUPERUSER NOCREATEDB) |
| B6 | Revocation lag across replicas | Accepted risk in 06-security §6.7 (≤ 30 s), owned by the identity-service maintainers |
| A7 | JWT header | RS256 only. `kid` required; the JWK must have `use=sig` (or no `use`) and `alg` RS256 (or no `alg`). Reject `jku`/`jwk`/`x5u`/`x5c`/`crit`. `typ` must be `JWT` or `at+jwt` (Hydra emits `JWT`). Token ≤ 8 KiB before parsing |
| A8 | JWKS cache | Single-flight refresh, 2 s timeout, 64 KiB / 20 keys max. Keep the current keys if a refresh fails. Periodic refresh every 5 min. Refresh on unknown kid at most once per 10 s. Never try all keys |
| A9 | Lifetime | Reject when `exp - iat > 10 min`, or when `iat`/`nbf` is in the future beyond the 30 s leeway. Exact `iss` match against `http://localhost:4444` (no trailing slash; tested) |
| A10 | Client body | Explicit `token_endpoint_auth_method: client_secret_basic`, `grant_types [client_credentials]`, `response_types [token]`, `audience [identity-service]`, `access_token_strategy: jwt`, no redirect_uris, jwks or lifespans. Hydra generates `client_id`. Rotate = `PATCH` JSON-patch on `/client_secret` + `/metadata/tokens_valid_after`, with a base64url 32-byte secret. `private_key_jwt` is the documented upgrade before external partners onboard |
| A11 | Hydra config | `oidc.dynamic_client_registration.enabled: false`, `oauth2.expose_internal_errors: false`, bcrypt cost 10. Rate-limit `/oauth2/token` per IP and per client at the ingress (prod; documented). Service startup refuses a non-https issuer, JWKS URL or admin URL unless `APP_ENV` is local or test |
| A12 | Rate limits | Per replica, documented. Plus a per-IP limiter on machine-plane 401s: 60/min → 429 |
| A13 | Plane binding | `/m2m` rejects any request carrying `Cookie: ory_kratos_session` or `X-Session-Token` (401) and has no CORS. `ValidatePolicies`: machine policies need a Scope and forbid Permission/Self/AllowAAL1, and non-machine policies forbid Scope. `/v1` rejects JWT-shaped bearers (three base64url segments starting with `eyJ`) with 401 **before** calling Kratos |
| A14 | Machine access log | A structured `m2m_access` log line with `client_id`, `jti`, method, route pattern, target id and status. Retention follows platform logging (documented) |
| A15 | Idempotency and secret display | Idempotency is bound to actor + request hash (existing table). The Hydra response body is never logged or persisted. Web: the secret lives in component state only, with no `useMutation` cache (or `gcTime: 0` + `reset()`), and is cleared when the dialog closes |
| A16 | Client-status cache | Negative results are cached too, for 30 s. Entries are never served past their TTL. Hydra 404 → 401; 5xx/timeout → 503 |
