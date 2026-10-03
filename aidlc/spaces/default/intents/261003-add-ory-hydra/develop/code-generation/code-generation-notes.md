# Code Generation Notes — H2 + H3 (M2M with Ory Hydra)

Units H2 (verifier, machine plane, `/m2m/v1`) and H3 (service-client API,
CLI, audit, idempotency/compensation) in `services/identity-service`. The
technical spec §6 (security review) was treated as authoritative over §1–§5
and the architecture doc. Not committed. `apps/`, `docs/` and `deploy/` were
not touched.

## Files

### New

| File | Contents |
| --- | --- |
| `internal/domain/machine/machine.go` (+ `_test`) | `Scope` enum (`customers:read`, `audit:read`), `Principal{ClientID, Scopes, TokenID}`, `ServiceClient`, `NewRegistration` (name `^[a-z0-9][a-z0-9-]*$`, 3–64 chars; owner email ≤ 254; scopes non-empty, known, unique, canonical order) |
| `internal/domain/audit/machine.go` (+ `_test`) | B1 allowlists: actions (`customer.disabled/enabled/sessions_revoked`, `admin.*`, `service_client.*`; `customer.pii.*` is never visible) and detail keys (`role`, `previous_role`, `scopes`, `name`, type-checked) |
| `internal/app/serviceclients.go` | `ServiceClientService`: List/Get/Create (reserve key, then Hydra, then tx with audit + complete key; on failure delete the client and release the key)/CreateAsSystem (CLI)/RotateSecret/Delete. Rotate and delete use `audited()` and purge the verifier cache |
| `internal/app/machine.go` | `MachineService`: PII-free customer status, allowlisted audit feed (scope re-checked in the use case) |
| `internal/adapter/hydra/http.go` | Client with no redirects, a 2 s timeout, a 1 MiB body cap and value-free errors |
| `internal/adapter/hydra/admin.go` | `app.ServiceClientAdmin`: explicit create body (A10); B2 `managedClient` filter on Get/List/status; paged List; PATCH rotate; Delete; `Ready` |
| `internal/adapter/hydra/jwks.go` | JWKS key set (A8) |
| `internal/adapter/hydra/clientcache.go` | Client-status LRU, positive and negative entries, TTL ≤ 30 s, invalidation generation (A16) |
| `internal/adapter/hydra/verifier.go` | `app.MachineTokenVerifier` (A7, A9, B2, B3); `Run` = periodic JWKS refresh every 5 min |
| `internal/adapter/hydra/{verifier,admin}_test.go` | Unit tests: local RSA keys, httptest JWKS and Hydra admin |
| `internal/adapter/hydra/hydra_integration_test.go` | Real Hydra tests (see "Integration tests" below) |
| `internal/adapter/httpapi/machine.go` | `MachineGuard` (A12, A13, A14), `looksLikeJWT`, limiter constructors |
| `internal/adapter/httpapi/server_machine.go` | Handlers for the 7 new operations |
| `internal/adapter/httpapi/machine_test.go` | Machine auth matrix, plane binding, CORS, limiters, log line, audit feed, policy validation, create/rotate over HTTP |
| `internal/adapter/postgres/audit_machine_integration_test.go` | SQL allowlist, exact paging, LIKE escaping |
| `internal/testutil/machine.go` | Fakes: `MachineVerifier`, `ServiceClients` |
| `internal/e2e/m2m_integration_test.go` | Full flow against real Hydra, Kratos and Keto |

### Modified

| File | Change |
| --- | --- |
| `internal/adapter/httpapi/gen/api.gen.go` | Regenerated (`go tool oapi-codegen …`); the contract itself was not changed |
| `db/queries/audit.sql`, `sqlcgen/audit.sql.go` | Optional `actions` / `action_patterns` filter (regenerated with `go tool sqlc generate`) |
| `internal/adapter/postgres/repo.go` | Passes the allowlist; `likeEscape` for prefixes |
| `internal/adapter/httpapi/policy.go` | `Policy.Scope`; 7 new entries. `ValidatePolicies`: `/m2m/v1/*` needs `PlaneMachine` with a valid scope and no Permission/Self/AllowAAL1, and non-machine routes may not declare a Scope |
| `internal/adapter/httpapi/context.go` | `PlaneMachine`; machine principal + required scope in the request state; `MachineFrom` |
| `internal/adapter/httpapi/problem.go` | `invalid_token` (401), `insufficient_scope` (403); `WWW-Authenticate: Bearer realm=…, error=…[, scope=…]` on machine 401/403 |
| `internal/adapter/httpapi/middleware.go` | Guard hands `/m2m` to `MachineGuard` (503 if not configured); `/v1` rejects JWT-shaped bearers before Kratos; no CORS on `/m2m`; access log adds `client_id` |
| `internal/adapter/httpapi/router.go`, `server.go`, `bodycheck.go` | Machine deps; `Server.Machine` / `Server.ServiceClients`; strict body (unknown fields → 422) for the create request |
| `internal/app/ports.go`, `errors.go` | `MachineTokenVerifier`, `ServiceClientAdmin`, `NewServiceClient`; `ErrInvalidToken`, `ErrInsufficientScope` |
| `internal/domain/identity/identity.go` | `PermManageServiceClients` (added to `AllPermissions`) |
| `internal/domain/audit/audit.go` | `service_client.*` actions, `TargetServiceClient`, `Filter.Actions/ActionPrefixes` + `Matches` |
| `internal/platform/config.go` (+ test) | `HYDRA_ADMIN_URL`, `M2M_JWKS_URL`, `M2M_ISSUER`, `M2M_AUDIENCE`, `M2M_CLIENT_CACHE_TTL` (0, 30 s], `M2M_RATE_LIMIT_PER_MIN`. https is required for all three URLs unless `APP_ENV` is local or test; the issuer may not carry a query, fragment or user info |
| `internal/platform/logger.go` (+ test) | Redacts `client_secret`, `access_token`, `refresh_token`, `id_token`, `bearer`, `jwt`, `registration_access_token`, `secrets_system`, `hydra_secrets_system` |
| `cmd/identity-service/main.go` | Wiring, JWKS warm-up (warns on failure), `Run` loop, `clients create --name --owner --scope…` |
| `internal/testutil/fakes.go`, `itest/itest.go` | Fake audit repo honours the action filter; Hydra env, `ClientCredentialsToken`, `DeleteHydraClient` |
| `internal/e2e/e2e_integration_test.go` | Stack wires the machine plane; super_admin now has **6** permissions (was 5) |
| `internal/adapter/keto/keto_integration_test.go` | super_admin expectation includes `manage_service_clients` |
| `go.mod` / `go.sum` | **New direct dependency `github.com/go-jose/go-jose/v4 v4.1.5`** (Apache-2.0). It was already in the module graph as an indirect dependency at v4.1.4 through tooling; it is now a direct require at v4.1.5. No other new modules (`golang.org/x/sync/singleflight` and `golang-lru` were already deps) |

## How the §6 items are implemented

| Item | Implementation |
| --- | --- |
| B1 | `MachineAuditEvent` DTO. The action allowlist is applied **in SQL**, so paging stays exact, and is re-checked in the app. Detail keys are allowlisted and type-checked. No `request_id` or `client_ip` is exposed |
| B2 | Token rules: `sub == client_id`, non-empty `scp` array, `aud` contains the audience, and none of `nonce`/`at_hash`/`azp` (also `c_hash`). Client rules: `metadata.managed_by == "identity-service"`, `grant_types == [client_credentials]`, `audience == [identity-service]`, and the client_id in the response must equal the requested one. Malformed metadata counts as unmanaged |
| B3 | Rotate PATCHes `/client_secret` and `/metadata/tokens_valid_after = now (unix)`. The verifier rejects `iat < tokens_valid_after`, and the rotating replica purges its cache entry |
| A7 | Size ≤ 8 KiB is checked first. My own header pre-parse, done before go-jose sees the token, requires: compact form with three non-empty base64url segments; `alg == RS256`; a string `kid`; `typ` ∈ {JWT, at+jwt, application/at+jwt}, case-insensitive. It rejects `jku`, `jwk`, `x5u`, `x5c`, `x5t`, `x5t#S256`, `crit`, `b64`, `zip`, `enc`. go-jose then parses with RS256 only. JWK filter: `use` sig/absent, `alg` RS256/absent |
| A8 | Single-flight refresh, detached from the request, 2 s timeout. Limits: 64 KiB and 20 keys. Current keys are kept if a refresh fails. Periodic refresh every 5 min. Unknown-kid refresh at most once per 10 s. Lookup is by kid only, never all keys |
| A9 | 30 s leeway for exp, nbf and iat. Rejected when `exp - iat > 10 min` or `exp < iat`. `iss` must match exactly (tested in unit and integration with a trailing slash) |
| A10 | Explicit create body (asserted field by field in the unit test). Hydra generates the client_id. The rotated secret is base64url of 32 random bytes |
| A12 | Per-client limiter: 600/min (configurable). Per-IP limiter: 60 machine-plane 401s per minute → 429. Both per replica |
| A13 | `/m2m` returns 401 on `Cookie: ory_kratos_session` or `X-Session-Token`. `/m2m` has no CORS. `/v1` returns 401 for JWT-shaped bearers before calling Kratos (tested: Kratos fake gets 0 calls). `ValidatePolicies` rules as above |
| A14 | `m2m_access` line with `client_id`, `jti`, `method`, `route` pattern, `target_id`, `status`, `request_id`. Emitted for every request that passes token verification |
| A15 | Idempotency uses the existing table keyed by actor + request hash. The stored body has no secret, and the hash is tagged with the operation. The Hydra response body is never logged, persisted or put in errors. The create response's `registration_access_token` is ignored |
| A16 | Positive and negative status entries, 30 s TTL, never served past TTL. Hydra 404/4xx → 401. 5xx/timeout → 503, and errors are not cached |

## Decisions and deviations

1. **`machine.Principal.TokenID` added.** The spec lists only `{ClientID, Scopes}`. The jti is needed for the A14 log line.
2. **`MachineTokenVerifier.Invalidate(clientID)` added to the port.** This mirrors `SessionVerifier.Invalidate`, and rotate/delete need it to purge the local cache.
3. **Problem codes on machine-plane 401:**
   - No `Authorization` header → `unauthenticated` with `WWW-Authenticate: Bearer realm="identity-service"` and no error attribute, per RFC 6750 §3.1.
   - Every other 401, including a session credential present, a non-Bearer scheme or multiple `Authorization` headers → `invalid_token` with `error="invalid_token"`.
4. **Granted scopes = token `scp` ∩ the client's currently registered scopes.** This is defence in depth: a token minted before a scope change cannot exceed the registration. Unknown scopes are dropped.
5. **`jti` is required, RSA keys must be ≥ 2048 bits, and a duplicate `kid` in the JWKS drops that kid.** These are hardening beyond the spec.
6. **Unknown kid when the refresh fails because Hydra is unreachable → 503, not 401.** With no keys at all, the rate-limited window also returns 503.
7. **The per-IP failure limiter blocks the IP, not just failures.** Once an IP exceeds 60 failed authentications in a minute, every machine request from it gets 429 for the rest of the window, valid tokens included.
8. **Rate limiters reuse `app.RateLimiter`.** They are keyed by name-based UUIDv5 of the client_id or IP, so raw values are not held as keys.
9. **The audit allowlist is pushed into SQL** (new nullable `actions` / `action_patterns` parameters, sqlc regenerated). Filtering after the query would break paging.
10. **Hydra cannot filter by metadata,** so List pages through all clients (500 per page, at most 20 pages) and filters in the service. Results are sorted newest first.
11. **Rotate is a PATCH (JSON patch) per §6 A10,** not the PUT described in architecture-doc §4.
12. **If the create response does not match the registration** (unmanaged, or no secret), the adapter deletes the client and returns an error.
13. **`created_by` is stored in Hydra metadata.** The CLI stores the nil UUID (system actor) and its audit row uses `request_id = "cli-clients-create"`.
14. **Hydra is not added to `/readyz`, deliberately.** A Hydra outage must not take the customer and admin planes out of rotation. The machine plane answers 503 instead. Startup does not fail if the JWKS warm-up fails; it logs a warning.
15. **Known limit (B3): `tokens_valid_after` has 1 s resolution.** Rejection is `iat < tva`. A token issued in the same second as the rotation, but before the PATCH, is still accepted until it expires (≤ 5 min). Hydra rejects the old secret right after the PATCH, so the window is at most one second of issuance. Using `iat <= tva` would instead reject legitimate tokens obtained in the same second after rotation.
16. **Scope for `/m2m/v1/audit-events` route matching:** `ListMachineAuditEvents` uses the same filters and cursor format as the admin list.

## Infra notes (no change made)

- No compose/Hydra config change was needed. Inside compose, the JWKS (`http://hydra:4444`) and issuer (`http://localhost:4444`) were verified working: warm-up succeeded and a container smoke test passed (404 for an unknown id, 403 with challenge, 401 for a JWT on `/v1/me`, no CORS headers, `m2m_access` lines logged).
- Production: https URLs are enforced by config. A NetworkPolicy is needed for the admin API (§6 B4). Token-endpoint rate limiting belongs at the ingress (A11).

## Verification (all run on 2026-10-03, stack up)

| Command (in `services/identity-service` unless noted) | Result |
| --- | --- |
| `go build ./...` | OK |
| `go vet ./... && go vet -tags integration ./...` | OK |
| `gofmt -l internal cmd` | clean |
| `go test -race ./...` | all packages `ok` |
| `go test -race -count=1 -tags integration ./...` | all packages `ok` (after updating the Keto permission table and the e2e permission count for the new permit) |
| CLI smoke: `identity-service clients create --name cli-smoke --owner … --scope customers:read --scope audit:read` (APP_ENV=local) | Secret printed once. Audit row `service_client.created` with actor `00000000-…`. Invalid input and an http admin URL in production are refused. The client was deleted afterwards |
| `make up` (repo root) | exit 0; every container healthy; `make health` all ok |

### Unit tests covering §5/§6

The verifier tests cover:
- `alg` none (with and without a signature), HS256 keyed with the RSA modulus, RS512, PS256, ES256, missing, array;
- `kid` missing, empty or numeric;
- an unknown kid refreshes once, then a flood of 20 unknown kids within 10 s causes no further fetches, and a rolled-over key is picked up after the interval;
- concurrent first use fetches once (single-flight);
- `jku`, `jwk`, `x5u`, `x5c`, `x5t`, `crit`, `b64` and `zip` headers;
- `typ` missing, JOSE or id_token+jwt (rejected), and the accepted variants;
- the 8 KiB cap, applied before any key lookup;
- tampered signature, swapped payload, and a wrong key under a known kid;
- `iss`: trailing slash, https, missing;
- `aud`: missing, empty, other, numeric, and the string or multi-value forms that are accepted;
- `exp`, `nbf` and `iat` inside and outside the 30 s leeway, a lifetime over 10 min, and `exp < iat`;
- `sub ≠ client_id`, client_id missing or too long;
- `nonce`, `at_hash` or `azp` present;
- `scp` missing, empty, a string or null; `jti` missing;
- client status: unmanaged, missing or malformed metadata, extra or other grant, extra or other audience, client_id mismatch;
- `tokens_valid_after` (equal is accepted, later rejects), negative cache plus TTL, local purge;
- Hydra down: cached entry ok within TTL, 503 after TTL, immediate recovery;
- JWKS down → 503; keys survive a failed refresh;
- JWKS parsing limits: 20 keys, 64 KiB, use=enc, RS512, a 1024-bit key, oct and unknown kty, duplicate kid;
- errors carry no token contents.

The admin tests check the exact create and PATCH bodies, the managed filter, Link-header paging, path escaping, no redirects and value-free errors.

The remaining unit tests cover:
- policy validation;
- the machine HTTP matrix (with `WWW-Authenticate` and no Kratos calls);
- the audit allowlist (domain, app and HTTP);
- domain validation;
- the service-client use cases (idempotent replay without the secret, compensation, key release, rotate and delete audit and purge, and permission checks).

### Integration tests (real Hydra)

`adapter/hydra` covers:
- the full lifecycle, with a real token verified offline;
- rotation: the old secret gets 401 at the token endpoint and the old token gets 401 from the verifier;
- no audience → 401;
- an unregistered scope → Hydra 400;
- an exact-issuer check;
- an unmanaged client created directly in Hydra → 401.

`e2e/TestM2M_E2E` covers:
1. A super_admin (real Keto) creates a client through the service. The admin role gets 403.
2. A replay returns no secret, and the client appears in the list.
3. With a Hydra token, `/m2m/v1/customers/{id}` returns 200 with exactly 4 fields and no PII.
4. Wrong scope → 403 with `scope="audit:read"`.
5. The audit feed contains no `customer.pii.*` events and no disallowed detail keys.
6. A Kratos token or cookie on `/m2m` → 401.
7. A Hydra JWT on `/v1/me`, `/admin/v1/me` and `/admin/v1/customers` → 401.
8. A token with no audience → 401.
9. Rotate: the old secret gets 401 at the token endpoint and the old token gets 401 from the service; the new secret works.
10. Delete: 401 immediately and 401 at the token endpoint.
11. The audit trail has created, rotated and deleted.
12. No secrets or tokens appear in the logs, and the `m2m_access` line is present.

## Review fixes (round 1)

The code review findings are resolved as follows. These items supersede deviations 3, 7, 11 and 15 above.

| # | Fix | Files | Tests |
| --- | --- | --- | --- |
| 1 | The per-IP failure budget is charged only on rejections (cookie, missing or malformed header, invalid token). There is no Peek before verification, so a verified token is never refused. Over budget, rejections answer 429. 503s are not charged. Startup warns when `TRUSTED_PROXY_HOPS=0` outside local/test (`Config.Warnings`) | `httpapi/machine.go`, `platform/config.go`, `cmd/.../main.go` | `TestM2M_FailureLimiterPerIP` (valid token from a throttled IP → 200; 503s don't count), `TestWarningsTrustedProxyHops` |
| 2 | `getRaw` treats only 404 and 400 (malformed id) as "not found". Every other non-2xx is `ErrDependencyUnavailable` and is never cached. Unexpected Hydra statuses are 503s throughout | `hydra/admin.go`, `hydra/http.go` | `TestVerifyAdminNon404IsDependencyErrorAndNotCached`, `TestAdminNon404StatusesAreDependencyErrors` |
| 3 | identity-service now chooses the `client_id` (a random UUID; Hydra accepts it, and a duplicate gets 409 → `ErrConflict`, never deleted). The Hydra create runs on `context.WithoutCancel` with a 10 s timeout. On an ambiguous failure (`ErrDependencyUnavailable`: transport, timeout, 5xx) that id is deleted. If the delete also fails, `possible_orphaned_service_client` is logged with client_id, name and actor_id | `app/serviceclients.go`, `app/ports.go` (`NewServiceClient.ClientID`), `hydra/admin.go` | `TestM2MFR08_AmbiguousCreateFailureIsCompensated` (HTTP and CLI paths, plus the failed-compensation log), `…NonAmbiguousCreateFailureDeletesNothing`, `…CreateSurvivesRequestCancellation`, `TestAdminCreateConflictAndIDMismatch` |
| 4 | Rotate and delete use two-phase audit (see below). If the final insert fails, the new secret is still returned and `service_client_audit_incomplete` is logged | `app/serviceclients.go`, `domain/audit/audit.go` (4 new `service_client.*` actions, machine-visible by prefix) | `TestM2MFR10_RotateFinalAuditFailureStillReturnsSecret`, `…RotateHydraFailureIsRecorded`, `…RotateStartedRowFailureChangesNothing`, `TestM2MFR11_DeleteIsAuditedAndPurgesCache` (including `deletion_failed`); the e2e test checks all 5 actions |
| 5 | `tokens_valid_after = ceil(now)+1` (`app.TokensValidAfter`), compared as `iat < tva`. Clients must fetch a new token 1–2 s after a rotation and retry on 401 | `app/serviceclients.go` | `TestTokensValidAfterIsCeilPlusOne`, `TestVerifyTokenIssuedInRotationSecondIsRejected`; e2e: a token fetched just before rotate → 401 |
| 6 | `Invalidate` calls `sf.Forget(id)` and then invalidates the cache. `status()` records the cache generation before the shared lookup and repeats the lookup if it changed (at most 3 attempts, then 503) | `hydra/verifier.go` | `TestVerifyInvalidateDuringInflightLookup` (two callers share a blocked lookup; mutation-checked: it fails without the re-check) |
| 7 | Contract: `400` and `422` added to both `/m2m` routes; `invalid_request` added to the Problem codes; regenerated | `api/openapi/identity-service.v1.yaml`, `gen/api.gen.go` | build |
| 8 | A malformed `Authorization` header (wrong scheme, extra or inner spaces, scheme only, repeated header) → 400 `invalid_request` with `WWW-Authenticate: Bearer realm=…, error="invalid_request"` | `httpapi/machine.go`, `httpapi/problem.go` | `TestM2M_AuthMatrix` (5 cases); e2e |
| 9 | Wrong method on an existing `/m2m` path, after authentication → 405 with an `Allow` header | `httpapi/machine.go` | `TestM2M_AuthMatrix`, `TestM2M_MethodNotAllowedListsAllow`; e2e |
| 10 | S4: `metadata.integrity` is set atomically on create. Its value is `base64url(HMAC-SHA256(key, client_id\|sorted scopes\|audience\|created_by))`. `managed()` checks it with `hmac.Equal`, so a missing or wrong tag means unmanaged → 401 / 404. The tag is unchanged on rotate. `M2M_CLIENT_TAG_KEY` (base64, ≥ 32 bytes) is required by serve and by the CLI, and is redacted in logs | `hydra/admin.go`, `platform/config.go`, `platform/logger.go`, `cmd/.../main.go` | `TestVerifyClientIntegrityTag` (missing, garbage, wrong key, scopes or created_by changed, reordered scopes ok), `TestAdminCreateSendsExplicitBody`, `TestNewAdminRequiresTagKey`, config tests; integration: forged `managed_by` and forged integrity value created directly in Hydra → 401 |

### Two-phase audit for rotate and delete

1. Commit the `*_started` row (`secret_rotation_started` or `deletion_started`).
2. Make the detached Hydra call.
3. Purge the status cache. This also happens when the call failed, because the outcome may be unknown.
4. Insert the final row: `secret_rotated` or `deleted` on success, `secret_rotation_failed` or `deletion_failed` on failure.

### Decisions from the review

- **Item 3 deviation:** the review suggested listing clients by name and created_by after an ambiguous failure. Because identity-service now chooses the `client_id`, the compensation deletes that exact id instead. This is stricter: it cannot touch another client with the same name.
- **Item 9:** the customer and admin planes still answer 404 for a wrong method after authentication, as the existing `TestP4_UnmatchedRouteFailsClosed` asserts. Only `/m2m` was changed to 405, as requested. Aligning the other planes would be a separate change.
- **Item 10:** clients created before this change have no tag and are now invalid, as agreed.

### Verification (round 1)

- `go build ./...`, `go vet ./...` and `go vet -tags integration ./...`: OK.
- `gofmt`: clean.
- `go test -race ./...`: all packages `ok`.
- `go test -race -count=1 -tags integration ./...`: all packages `ok`.
- `make up` with `M2M_CLIENT_TAG_KEY` present in compose and `.env`: exit 0, identity-service healthy, no startup warnings (APP_ENV=local).
- Container smoke, with a client created by the CLI using the compose key: 404 for an unknown id (so the token verified), 400 for a malformed `Authorization` header, and 405 for a wrong method.
