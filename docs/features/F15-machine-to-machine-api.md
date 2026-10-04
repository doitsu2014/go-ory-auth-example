# F15 — Machine-to-machine API (`/m2m/v1`)

A partner service gets a short-lived RS256 JWT from Hydra with
`client_credentials`. It then calls the read-only `/m2m/v1` plane.
identity-service verifies the token **offline** against the JWKS, plus a
cached client-status lookup. It does not use token introspection. The
effective scopes are the token's `scp` intersected with the client's
**current** Hydra scopes.

## Actors and entry points

| Endpoint | Required scope | Returns |
| --- | --- | --- |
| Hydra `POST /oauth2/token` | — | JWT (TTL 5 min, `scp` list) |
| `GET /m2m/v1/customers/{id}` | `customers:read` | `{id, state, email_verified, created_at}`, no PII |
| `GET /m2m/v1/audit-events` | `audit:read` | allowlisted actions with reduced `details` |

## Functional diagram — MachineGuard

```mermaid
flowchart TD
  A["Request /m2m/v1/*"] --> B{"Kratos cookie or X-Session-Token?"}
  B -- yes --> X401a[401 invalid_token]
  B -- no --> C{"Authorization: Bearer, single header?"}
  C -- missing --> X401b[401 unauthenticated]
  C -- malformed --> X400[400 invalid_request]
  C -- ok --> H{"JWS header: RS256, kid, typ, no jku/jwk/x5u/x5c/crit/..."}
  H -- bad --> INV
  H -- ok --> K{"kid in JWKS cache?"}
  K -- no --> KR["refresh JWKS (≤ 1 per 10 s)"]
  KR -- "still unknown" --> INV
  KR -- "fetch fails" --> X503[503]
  K -- yes --> S{"signature + claims: iss, aud, exp/iat/nbf ±30 s, lifetime ≤ 10 min, jti, sub = client_id, scp"}
  KR -- found --> S
  S -- bad --> INV
  S -- ok --> ST{"client status (cache ≤ 30 s, else Hydra GET /admin/clients/id)"}
  ST -- "Hydra down" --> X503
  ST -- "not managed / missing" --> INV
  ST -- ok --> RV{"iat ≥ tokens_valid_after?"}
  RV -- no --> INV[401 invalid_token]
  X401a & X401b & X400 & INV -. "counted by per-IP failure limiter 60/min" .-> FL["over budget → 429 rate_limited"]
  RV -- yes --> SC["effective scopes = scp ∩ client scopes"]
  SC --> R{"route + policy?"}
  R -- no --> X404[404 / 405]
  R -- yes --> P{"has route scope?"}
  P -- no --> X403[403 insufficient_scope]
  P -- yes --> CL{"ClientLimiter 600/min"}
  CL -- exceeded --> X429[429]
  CL -- ok --> HD[Handler + m2m_access log line]
```

## Sequence

```mermaid
sequenceDiagram
  autonumber
  participant PS as Partner service
  participant HP as Hydra public :4444
  participant IS as identity-service :8080
  participant HA as Hydra admin :4445
  participant KA as Kratos admin
  participant DB as identity DB

  PS->>HP: POST /oauth2/token (Basic client_id:secret) grant_type=client_credentials&scope=customers:read audit:read&audience=identity-service
  HP-->>PS: access_token (RS256 JWT, scp list, exp 5 min)
  PS->>IS: GET /m2m/v1/customers/{id} + Authorization: Bearer JWT
  IS->>IS: checkHeader, keys.key(kid)
  opt unknown kid
    IS->>HP: GET /.well-known/jwks.json
  end
  IS->>IS: verify RS256 + claims
  alt status cached
    IS->>IS: statusCache hit
  else miss
    IS->>HA: GET /admin/clients/{client_id}
    HA-->>IS: client (metadata.integrity, scopes, tokens_valid_after)
  end
  IS->>IS: managed? iat ≥ tokens_valid_after? scopes = scp ∩ client scopes
  IS->>IS: route scope customers:read (403), ClientLimiter (429)
  IS->>KA: GET /admin/identities/{id}
  alt customer
    IS-->>PS: 200 {id, state, email_verified, created_at}
  else not found or not customer
    IS-->>PS: 404
  end
  Note over IS: log m2m_access {client_id, jti, route, target_id, status} (not in audit_event)

  PS->>IS: GET /m2m/v1/audit-events?target_type&page_size&page_token
  IS->>DB: SELECT audit_event WHERE action IN (customer.disabled, enabled, sessions_revoked) OR LIKE admin.% OR service_client.%
  IS->>IS: drop customer.pii.*, reduce details to {role, previous_role, scopes, name}
  IS-->>PS: 200 {items, next_page_token}
```

## Code references

- `services/identity-service/internal/adapter/httpapi/machine.go:81` — `MachineGuard.serve`
- `services/identity-service/internal/adapter/hydra/verifier.go:106` — `Verify`, `:164` header, `:219` claims, `:254` status
- `services/identity-service/internal/adapter/hydra/jwks.go:61` — key cache and refresh
- `services/identity-service/internal/adapter/hydra/clientcache.go`
- `services/identity-service/internal/app/machine.go:63` — `Customer`, `:84` `AuditEvents`
- `services/identity-service/internal/domain/audit/machine.go:13` — allowlist filter
- `services/identity-service/internal/adapter/httpapi/policy.go:75` — m2m scopes
- `deploy/ory/hydra/hydra.yml`

## Related

[ADR-0012](../adr/0012-hydra-for-machine-to-machine.md) ·
[09 Machine access](../architecture/09-machine-access.md)
