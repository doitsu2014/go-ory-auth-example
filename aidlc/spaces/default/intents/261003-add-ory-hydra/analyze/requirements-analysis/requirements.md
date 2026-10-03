# Requirements — Machine-to-machine access (Hydra)

## Functional

| ID | Requirement | Acceptance criterion |
| --- | --- | --- |
| M2M-FR-01 | Hydra issues client_credentials tokens | `POST :4444/oauth2/token` with `client_secret_basic` returns a JWT (`token_type=bearer`, ≤ 5 min) when scope ⊆ client scopes and `audience=identity-service`; otherwise 400 `invalid_scope` / `invalid_request` |
| M2M-FR-02 | Machine API plane `/m2m/v1/*` accepts only Hydra JWT Bearer | Kratos session token or cookie → 401; Hydra JWT on `/v1` or `/admin/v1` → 401 |
| M2M-FR-03 | Token verification | RS256 signature against Hydra JWKS (cached, refreshed on unknown `kid`, ≤ 1 refresh / 10 s), `iss` = configured issuer, `aud` contains `identity-service`, `exp`/`nbf` with ≤ 30 s leeway, `alg` allowlist (RS256/ES256, never `none`/HS*) → else 401 `invalid_token` with `WWW-Authenticate: Bearer error="invalid_token"` |
| M2M-FR-04 | Per-route scope | Startup fails if an `/m2m/v1` route has no scope policy; missing scope → 403 `insufficient_scope` (`WWW-Authenticate` with `scope=`) |
| M2M-FR-05 | `GET /m2m/v1/customers/{id}` (`customers:read`) | id, state, email_verified, created_at — **no email, no name, no PII**; non-customer → 404 |
| M2M-FR-06 | `GET /m2m/v1/audit-events` (`audit:read`) | Same filters and paging as the admin audit list |
| M2M-FR-07 | Client status check | A deleted client's still-valid JWT is rejected within ≤ 30 s (cached Hydra admin lookup); Hydra unreachable and no cache entry → 503 |
| M2M-FR-08 | super_admins create a service client (`POST /admin/v1/service-clients`, `Idempotency-Key`) | name, owner contact, scopes ⊆ {customers:read, audit:read}; grant `client_credentials` only, audience `identity-service`, JWT strategy; `client_secret` returned **once** in the 201 body; audit `service_client.created` |
| M2M-FR-09 | List / get service clients | Only clients created by identity-service (metadata marker); never returns secrets |
| M2M-FR-10 | Rotate secret | New 32-byte random secret, returned once; old secret stops working immediately; audit |
| M2M-FR-11 | Delete client | Hydra client deleted, local cache purged, audit; tokens stop working ≤ 30 s on other replicas |
| M2M-FR-12 | Admin web page "Service clients" (super_admin) | list, create (secret shown once with copy + warning), rotate, delete with confirm; tests |
| M2M-FR-13 | CLI `identity-service clients create --name --owner --scope` | prints the secret once to stdout; audited with a system actor |
| M2M-FR-14 | Compose | Hydra + migrate + DB provisioning on existing volumes; `make up` healthy; admin port 127.0.0.1 only |

## Non-functional

| ID | Requirement | Acceptance criterion |
| --- | --- | --- |
| M2M-NFR-01 | Least privilege | Hydra admin API only reachable by identity-service (127.0.0.1 on host); DB role `hydra` owns only DB `hydra` |
| M2M-NFR-02 | Secrets | Client secrets, `Authorization` headers and tokens are redacted in logs; Hydra `SECRETS_SYSTEM` from env, ≥ 32 chars |
| M2M-NFR-03 | Observability | Request log and metrics label machine calls with `client_id` (never the token); audit stays for admin mutations |
| M2M-NFR-04 | Rate limit | Per-client 600 requests/min → 429 |
| M2M-NFR-05 | Performance | Verification is local (no network) when JWKS and client status are cached |
