# F04 — Customer password recovery

A customer who forgot their password asks for a code, enters it, and sets a
new password. identity-service starts the Kratos recovery flow with the
customer's handle, or with a decoy if the address is unknown. It returns an
**encrypted** `recovery_id`, so the client never learns the Kratos flow id.
The final password change goes straight to Kratos, using the privileged
session that the recovery issues.

## Actors and entry points

| Actor | Client | Entry point |
| --- | --- | --- |
| Customer | Mobile app `ForgotPasswordScreen` (email → code → new password) | `POST /v1/auth/recovery`, `POST /v1/auth/recovery/code`, Kratos settings flow |

## Functional diagram

```mermaid
flowchart TD
  A["Enter email / phone"] --> B["POST /v1/auth/recovery"]
  B --> C{"SignIn limiter + parse"}
  C -- fail --> E1[422 / 429]
  C -- ok --> D{"Recovery target"}
  D -- "vault row: bound, or phase complete, or phone" --> T1[handle]
  D -- "row unbound, legacy phase" --> T2["Kratos admin lookup → handle or legacy email"]
  D -- "no row, legacy email" --> T3[legacy email]
  D -- unknown --> T4[random decoy]
  T1 & T2 & T3 & T4 --> K["Kratos recovery flow, method code"]
  K --> R["200 {recovery_id = encrypt(flow id)} - same shape for all targets"]
  K -.->|"only real identities"| CR["Courier recovery_code_valid → email / SMS"]
  R --> F["Enter code → POST /v1/auth/recovery/code"]
  F --> G{"code format, recovery_id decrypts to UUID?"}
  G -- no --> E2["422 code / recovery_id invalid"]
  G -- yes --> H{"Kratos accepts code?"}
  H -- wrong --> E3[400 auth_flow_rejected]
  H -- expired --> E4[410 auth_flow_expired]
  H -- ok --> I["200 {session_token (privileged), settings_flow_id}<br/>Kratos revokes other sessions"]
  I --> J["App → Kratos settings: new password"]
  J --> L["Store token, whoami → signed in"]
  F -.->|"user leaves"| AB["abandonRecovery: DELETE /self-service/logout/api"]
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
  participant KA as Kratos admin

  MA->>IS: POST /v1/auth/recovery {login:{type,value}}
  IS->>IS: SignIn + Net limiters (429), parse (422)
  IS->>OB: transit/hmac → lookup_key
  IS->>DB: SELECT login_identifier WHERE lookup_key
  opt row unbound and legacy phase
    IS->>KA: GET /admin/identities?credentials_identifier=handle
  end
  IS->>IS: target = handle | legacy email | random decoy
  IS->>KP: GET /self-service/recovery/api
  IS->>KP: POST /self-service/recovery?flow=id {method:code, email: target}
  KP-->>IS: 200 flow sent_email (identical for unknown targets)
  IS->>OB: transit/encrypt identity-login-kek (AD identity-service/recovery-flow/v1, flow id)
  OB-->>IS: vault:vN:...
  IS-->>MA: 200 {recovery_id}
  KP-)IS: courier recovery_code_valid (real identities only, see F01)

  MA->>IS: POST /v1/auth/recovery/code {recovery_id, code}
  IS->>IS: SignIn limiter, ValidCode, sealed-ref format (422)
  IS->>OB: transit/decrypt recovery_id → flow id (must be UUID, else 422)
  IS->>KP: POST /self-service/recovery?flow=flow_id {method:code, code}
  alt code valid
    KP->>KP: after hook revoke_active_sessions
    KP-->>IS: 200 continue_with [set_ory_session_token, show_settings_ui]
    IS-->>MA: 200 {session_token, settings_flow_id}
  else wrong code
    IS-->>MA: 400 auth_flow_rejected
  else expired
    IS-->>MA: 410 auth_flow_expired
  end

  opt settings_flow_id empty or expired
    MA->>KP: GET /self-service/settings/api + X-Session-Token
  end
  MA->>KP: POST /self-service/settings?flow=sid {method:password, password} + X-Session-Token
  KP-->>MA: 200 settings success
  MA->>MA: tokens.write(recovery token)
  MA->>KP: GET /sessions/whoami → signedIn
```

## Errors and outcomes

| Condition | Response |
| --- | --- |
| Unknown address | 200 with a `recovery_id` anyway (decoy), and no message is sent |
| Malformed code or tampered `recovery_id` | 422 `validation_failed` (`code`/`invalid_format`, `recovery_id`/`invalid`) |
| Wrong code | 400 `auth_flow_rejected` |
| Flow expired | 410 `auth_flow_expired` |
| Rate limit (per IP, net) | 429. **The per-account limiter is not applied to recovery.** |

## Code references

- `apps/mobile/lib/features/auth/presentation/forgot_password_screen.dart:74`
- `apps/mobile/lib/features/auth/data/auth_repository.dart:169` — request, `:183` complete, `:225` abandon
- `services/identity-service/internal/app/customerauth.go:269` — `StartRecovery`, `:297` `recoveryTarget`, `:338` `SubmitRecoveryCode`
- `services/identity-service/internal/adapter/kratos/selfservice.go:208` — recovery flow
- `services/identity-service/internal/adapter/httpapi/server.go:475`
- `deploy/ory/kratos/kratos.yml.tmpl:64` — recovery `use: code`, `revoke_active_sessions`

## Related

[ADR-0014](../adr/0014-customer-login-through-identity-service.md) ·
[10 §10.4](../architecture/10-pseudonymous-login.md)
