# F06 — Request authentication (all planes)

Every request to identity-service passes through one middleware chain and one
**Guard**. The Guard picks a *plane* from the path, then checks the
credential, the population, the route policy, and the permission. This
cross-cutting flow comes before every feature on F07–F15.

## Actors and entry points

| Plane | Path prefix | Credential | Verified by |
| --- | --- | --- | --- |
| Public | `/v1/auth/*` | none (client IP required) | rate limiters ([F03](F03-customer-login.md)) |
| Customer | `/v1`, `/v1/*` | `Authorization: Bearer <session_token>` | Kratos `GET /sessions/whoami` |
| Admin | `/admin/v1/*` | `ory_kratos_session` cookie | Kratos whoami + AdminGate + CSRF + Keto |
| Machine | `/m2m/v1/*` | `Authorization: Bearer <JWT>` | Hydra JWKS ([F15](F15-machine-to-machine-api.md)) |
| Webhook | `:8081 /internal/hooks/kratos/*` | `Authorization: <api key>` | constant-time compare |

## Functional diagram

```mermaid
flowchart TD
  A[Request on :8080] --> B["requestState: X-Request-Id kept or UUIDv7"]
  B --> C["recoverer, accessLog, limits: body 1 MiB, timeout 10 s"]
  C --> D[cors + securityHeaders]
  D --> E{"planeOf(path)"}
  E -- "/v1/auth/*" --> P0{"Route + public policy?"}
  P0 -- "no route" --> X404
  P0 -- "policy not public" --> X500
  P0 -- yes --> P1{Client IP known?}
  P1 -- no --> X400[400 invalid_request]
  P1 -- yes --> PH["handler with PublicFlowClient (rate limits in F01/F03/F04)"]
  E -- "/m2m/v1/*" --> M1["MachineGuard: verify JWT, see F15"]
  E -- "/v1/*" --> C1{"Bearer token, no cookie, no X-Session-Token, not a JWT?"}
  E -- "/admin/v1/*" --> A1{"ory_kratos_session cookie, no Authorization / X-Session-Token?"}
  C1 -- no --> X401[401 unauthenticated]
  A1 -- no --> X401
  C1 -- yes --> V
  A1 -- yes --> V
  V{"SessionCache hit?"} -- yes --> PR
  V -- no --> W["Kratos GET /sessions/whoami"]
  W -- "403 session_aal2_required" --> X403a[403 aal2_required]
  W -- "other 4xx" --> X401
  W -- "5xx / timeout" --> X503[503 dependency_unavailable]
  W -- 200 --> S{"active, not expired, identity active?"}
  S -- no --> X401
  S -- yes --> K{"schema_id known?"}
  K -- no --> X403[403 forbidden]
  K -- yes --> CP["Cache principal, TTL = min(30 s, expires_at)"]
  CP --> PR{"Population matches plane?"}
  PR -- "customer on /admin" --> X403n[403 not_admin]
  PR -- "admin on /v1" --> X403
  PR -- yes --> R{"Route + policy match?"}
  R -- "no route" --> X404[404 not_found]
  R -- "policy missing" --> X500[500 internal - fail closed]
  R -- yes --> G{Admin plane?}
  G -- yes --> AG["AdminGate: AAL2, MFA enrolled, CSRF/Origin on mutations, see F09"]
  AG --> KT
  G -- no --> KT{"policy.Permission set?"}
  KT -- no --> H[strictBody check, then handler]
  KT -- yes --> KC["Keto check namespace#relation@subject"]
  KC -- denied --> X403
  KC -- allowed --> H
```

## Sequence — customer request (`/v1/*`)

```mermaid
sequenceDiagram
  autonumber
  participant MA as Mobile app
  participant IS as identity-service :8080
  participant SC as SessionCache (in-process LRU)
  participant KP as Kratos public :4433

  MA->>IS: GET /v1/me + Authorization: Bearer session_token
  Note over IS: planeOf = Customer. Reject cookie, X-Session-Token, JWT-shaped bearer (401)
  IS->>SC: Get(SHA-256(kind ‖ token))
  alt cache hit and entry fresh
    SC-->>IS: Principal
  else miss
    IS->>SC: Generation() = startGen
    IS->>KP: GET /sessions/whoami + X-Session-Token (2 s timeout, 1 retry on 5xx)
    alt 200 session active and identity.state = active
      KP-->>IS: session {identity{schema_id, traits.login_id}, aal, expires_at}
      IS->>IS: toPrincipal (kind from schema_id, LoginID, EmailVerified)
      IS->>SC: PutIfFresh(token, principal, startGen) TTL ≤ 30 s
    else 403 session_aal2_required
      IS-->>MA: 403 aal2_required
    else other 4xx or inactive / expired / disabled
      IS-->>MA: 401 unauthenticated + WWW-Authenticate
    else 5xx or timeout
      IS-->>MA: 503 dependency_unavailable
    end
  end
  IS->>IS: setActor(principal, request_id, client IP)
  IS->>IS: kind == customer ? else 403 forbidden
  IS->>IS: Router.Match + RoutePolicies (customer routes are Self, no Keto)
  IS->>IS: strictBodyMiddleware (unknown fields → 422)
  IS-->>MA: handler response
  opt response 401 for the current token
    MA->>MA: single-flight onUnauthorized → AuthController.forget() → Unauthenticated
  end
```

## Sequence — cache invalidation

```mermaid
sequenceDiagram
  autonumber
  participant AD as Admin action (disable / revoke / AdminGate sweep)
  participant IS as identity-service
  participant SC as SessionCache
  AD->>IS: CustomerService.Sessions / AdminGate deactivate
  IS->>SC: Invalidate(identity_id)
  SC->>SC: generation++, record epoch, drop LRU entries for identity
  Note over SC: A whoami already in flight with older startGen is NOT cached (epoch guard)
```

## Errors and outcomes

| Condition | Status / problem |
| --- | --- |
| No credential or wrong credential type for the plane | 401 `unauthenticated` |
| Session inactive, expired, or identity disabled | 401 `unauthenticated` |
| Kratos requires AAL2 | 403 `aal2_required` |
| Unknown schema, or admin on `/v1` | 403 `forbidden` |
| Customer on `/admin/v1` | 403 `not_admin` |
| No route | 404 `not_found` |
| Policy missing | 500 `internal` (fail closed); the service refuses to start if policies are invalid |
| Body > 1 MiB, or unknown field | 422 `validation_failed` |
| Kratos 5xx or timeout | 503 `dependency_unavailable` |

## Code references

- `services/identity-service/internal/adapter/httpapi/router.go:87` — middleware order
- `services/identity-service/internal/adapter/httpapi/context.go:36` — `planeOf`
- `services/identity-service/internal/adapter/httpapi/middleware.go:215` — `Guard.Middleware`
- `services/identity-service/internal/adapter/httpapi/middleware.go:347` — `credentialFor`
- `services/identity-service/internal/adapter/kratos/verifier.go:42` — whoami and status mapping
- `services/identity-service/internal/adapter/kratos/cache.go:64` — cache key, `PutIfFresh`, `Invalidate`
- `services/identity-service/internal/adapter/kratos/model.go:104` — `toPrincipal`
- `services/identity-service/internal/adapter/httpapi/policy.go:43` — route policies
- `services/identity-service/internal/adapter/httpapi/bodycheck.go:92` — strict body
- `services/identity-service/internal/adapter/httpapi/problem.go:51` — problem → status
- `apps/mobile/lib/core/network/interceptors.dart:86` — bearer + 401 handling

## Related

[ADR-0006](../adr/0006-session-validation-in-service.md) ·
[ADR-0005](../adr/0005-keto-for-authorization.md) ·
[03 §3.5](../architecture/03-auth-flows.md)
