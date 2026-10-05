# F03 — Customer login

The customer signs in with email or phone plus password. identity-service turns
the address into a handle and runs the Kratos native login flow. The address
itself never reaches Kratos. Unknown addresses get a random **decoy** handle,
so the response and timing look the same as a wrong password.

## Actors and entry points

| Actor | Client | Entry point |
| --- | --- | --- |
| Customer | Mobile app `SignInScreen` | `POST /v1/auth/login {login:{type,value}, password}` |
| Kratos | webhook | `POST :8081/internal/hooks/kratos/after-login` |

## Functional diagram

```mermaid
flowchart TD
  A["POST /v1/auth/login"] --> B{"Password present and ≤ 1024 bytes?"}
  B -- no --> E422[422 validation_failed]
  B -- yes --> C{"SignIn limiter 20/min/IP + Net limiter"}
  C -- exceeded --> E429[429 rate_limited]
  C -- ok --> D{"Parse + normalise login"}
  D -- invalid --> E422
  D -- ok --> F["lookup_key = HMAC(login) via OpenBao"]
  F --> G{"Vault row?"}
  G -- yes --> H[Candidates: handle]
  G -- no --> I
  H --> I{"Legacy transition, email, and row missing or unbound?"}
  I -- yes --> J[Append legacy plaintext email]
  I -- no --> K0{"Candidates empty?"}
  J --> K0
  K0 -- yes --> K[Candidates: random decoy handle]
  K0 -- no --> L
  K --> L{"Account limiter 10 per 15 min per lookup_key"}
  L -- exceeded --> E429
  L -- ok --> M["Kratos native login with next candidate"]
  M --> O{"Credentials valid?"}
  O -- "no, 4000006 and more candidates" --> M
  O -- no --> R400["400 auth_flow_rejected form/4000006"]
  O -- yes --> N{"after-login hook: customer + api flow?"}
  N -- no --> R400a["400 auth_flow_rejected form/4000001 (loop stops)"]
  N -- yes --> S["200 {session_token, session}"]
  S --> T["App stores token → Authenticated"]
```

## Sequence

```mermaid
sequenceDiagram
  autonumber
  participant MA as Mobile app
  participant IS as identity-service :8080
  participant OB as OpenBao transit
  participant DB as identity DB
  participant KP as Kratos public
  participant WH as identity-service :8081 (hooks)

  MA->>IS: POST /v1/auth/login {login:{type,value}, password}
  IS->>IS: validatePassword, SignIn + Net limiters, parse (422 / 429)
  IS->>OB: transit/hmac/identity-login-pseudonym
  OB-->>IS: lookup_key
  IS->>DB: SELECT login_identifier WHERE lookup_key
  IS->>IS: candidates = [handle if row] + [legacy email if transition, email, row missing or unbound]
  IS->>IS: no candidates → [random decoy]
  IS->>IS: AccountLimiter.Allow(lookup_key) - counts every attempt (429)
  loop each candidate while Kratos answers 4000006
    IS->>KP: GET /self-service/login/api
    KP-->>IS: flow id
    IS->>KP: POST /self-service/login?flow=id {method:password, identifier: candidate, password}
    opt password accepted
      KP->>WH: POST /internal/hooks/kratos/after-login {identity_id, schema_id, flow_type}
      alt customer and flow_type = api
        WH-->>KP: 204
      else wrong population or flow type
        WH-->>KP: 403 message 4000001 (ends the loop)
      end
    end
  end
  alt success
    KP-->>IS: 200 {session_token, session}
    IS-->>MA: 200 CustomerAuthSession {session_token, session}
    MA->>MA: SecureTokenStore.write → AuthController.signedIn
  else wrong password or unknown address (decoy)
    KP-->>IS: 400 flow 4000006
    IS-->>MA: 400 auth_flow_rejected errors[{field:form, code:4000006}]
  else flow expired
    IS-->>MA: 410 auth_flow_expired
  else Kratos / OpenBao down
    IS-->>MA: 503 dependency_unavailable
  end
```

## Sequence — app start (session restore)

```mermaid
sequenceDiagram
  autonumber
  participant MA as Mobile app
  participant KP as Kratos public
  MA->>MA: read kratos_session_token from secure storage
  alt token present
    MA->>KP: GET /sessions/whoami + X-Session-Token
    alt 200
      KP-->>MA: session → Authenticated
    else 401
      KP-->>MA: 401 → wipe token → Unauthenticated
    end
  else no token
    MA->>MA: Unauthenticated → Sign-in screen
  end
```

## Errors and outcomes

| Condition | Response |
| --- | --- |
| Wrong password, unknown address | 400 `auth_flow_rejected` `form`/`4000006` (same for both) |
| Wrong population (admin account, browser flow) | 400 `auth_flow_rejected` `form`/`4000001` |
| Disabled account | 400 `auth_flow_rejected` (Kratos message id) |
| Unverified account | **not blocked**; writes are gated later ([F02](F02-customer-verification.md)) |
| Per-IP, per-net or per-account limit | 429 `rate_limited` |

Rate limiters are in-memory and kept **per replica**.

## Code references

- `apps/mobile/lib/features/auth/presentation/sign_in_screen.dart:50`
- `apps/mobile/lib/features/auth/data/auth_repository.dart:74` — restore, `:116` login
- `services/identity-service/internal/adapter/httpapi/server.go:439` — `CustomerLogin`
- `services/identity-service/internal/app/customerauth.go:170` — `candidates`, `:195` `Login`
- `services/identity-service/internal/app/login.go:56` — limiters
- `services/identity-service/internal/adapter/kratos/selfservice.go:168` — login flow
- `services/identity-service/internal/app/provisioning.go:41` — `LoginAllowed`
- `services/identity-service/internal/adapter/httpapi/router.go:259` — after-login hook
- `services/identity-service/internal/domain/login/pseudonym.go:86` — lookup input

## Related

[ADR-0014](../adr/0014-customer-login-through-identity-service.md) ·
[10 §10.1](../architecture/10-pseudonymous-login.md)
