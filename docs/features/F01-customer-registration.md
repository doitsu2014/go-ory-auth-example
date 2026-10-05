# F01 — Customer registration and code delivery

A customer signs up in the mobile app with an email address or phone number
and a password. identity-service stores the real address in its encrypted
**login vault** (`login_identifier`) and registers the customer in Kratos
under an opaque **handle** (`<52 base32 chars>@login.invalid`). Kratos never
sees the real address. When Kratos sends a verification code, it goes back
through identity-service, which decrypts the address and delivers by email or
SMS.

## Actors and entry points

| Actor | Client | Entry point |
| --- | --- | --- |
| Customer | Mobile app `SignUpScreen` | `POST /v1/auth/registration {login:{type,value}, password}` |
| Kratos | webhooks to `:8081` | `pre-registration`, `after-registration`, `courier` |

## Functional diagram

```mermaid
flowchart TD
  A["Customer submits login + password"] --> B{"Client validation: email regex / E.164"}
  B -- invalid --> B1[Field error in app]
  B -- ok --> C["POST /v1/auth/registration"]
  C --> D{"Password present and ≤ 1024 bytes?"}
  D -- no --> E422[422 validation_failed]
  D -- yes --> RL{"Register limiter 5/min/IP + net limiter"}
  RL -- exceeded --> E429[429 rate_limited]
  RL -- ok --> P{"Parse + normalise login. Phone needs an SMS channel"}
  P -- invalid --> E422
  P -- ok --> CL{"Vault row for lookup_key?"}
  CL -- yes --> H1[Reuse existing handle]
  CL -- no --> H2["New random handle, seal address, INSERT login_identifier"]
  H1 --> K["Kratos native registration with traits.login_id = handle"]
  H2 --> K
  K --> KV{"Kratos password policy: length ≥ 12, HIBP, similarity"}
  KV -- fails --> E400[400 auth_flow_rejected]
  KV -- ok --> PR{"pre-registration hook: handle resolvable, no legacy duplicate?"}
  PR -- "no (4049001 / 4049002 / 4000007)" --> E400
  PR -- yes --> UQ{"Identifier unique in Kratos?"}
  UQ -- "no (4000007)" --> E400
  UQ -- yes --> PER[Kratos persists identity]
  PER --> AR["after-registration hook (async): INSERT profile, bind vault row"]
  PER --> S["session hook: 200 session_token + verification_flow_id"]
  PER -.-> CR["Courier: verification_code_valid → F02 delivery"]
  S --> APP["App stores token in secure storage → Verify screen"]
```

## Sequence — registration

```mermaid
sequenceDiagram
  autonumber
  participant MA as Mobile app
  participant IS as identity-service :8080
  participant OB as OpenBao transit
  participant DB as identity DB
  participant KP as Kratos public
  participant KA as Kratos admin
  participant WH as identity-service :8081 (hooks)

  MA->>IS: POST /v1/auth/registration {login:{type,value}, password}
  IS->>IS: validatePassword, Register + Net limiters (429), parse/normalise (422)
  IS->>OB: POST transit/hmac/identity-login-pseudonym {"login-id/v1"‖kind‖value}
  OB-->>IS: lookup_key
  IS->>DB: SELECT login_identifier WHERE lookup_key
  alt no row
    IS->>IS: NewPseudonym → handle
    IS->>OB: transit/encrypt/identity-login-kek (AAD kind + handle)
    OB-->>IS: value_ct
    IS->>DB: INSERT login_identifier ON CONFLICT DO NOTHING (re-read on lost race)
  else row exists (bound or unbound)
    Note over IS: reuse handle - an already-registered address will fail at Kratos with 4000007
  end
  IS->>KP: GET /self-service/registration/api
  KP-->>IS: flow id
  IS->>KP: POST /self-service/registration?flow=id {method:password, password, traits:{login_id: handle}}
  KP->>KP: password policy (min 12, HIBP, similarity) - weak password rejected before any hook
  KP->>WH: POST /internal/hooks/kratos/pre-registration {schema_id, flow_type, login_id, email}
  WH->>WH: legacy email trait present → reject 4049001
  WH->>DB: SELECT login_identifier by handle (missing → 4049002)
  opt transition phase and kind = email
    WH->>OB: transit/decrypt value_ct
    WH->>KA: GET /admin/identities?credentials_identifier=email (customer found → 4000007)
  end
  WH->>DB: UPDATE login_identifier SET last_validated_at
  alt hook ok
    WH-->>KP: 200 {}
  else legacy / unresolved / duplicate
    WH-->>KP: 400 message 4049001 / 4049002 / 4000007 at #/traits/login_id
    KP-->>IS: 400 flow with errors
    IS-->>MA: 400 auth_flow_rejected errors[{field:login, code}]
  end
  KP->>KP: identifier uniqueness (4000007), persist identity
  KP-)WH: POST /internal/hooks/kratos/after-registration {identity_id, schema_id, login_id} (response ignored)
  WH->>DB: INSERT profile ON CONFLICT DO NOTHING
  WH->>DB: UPDATE login_identifier SET identity_id, bound_at (failure only logged)
  KP-->>IS: 200 {session_token, session, continue_with: show_verification_ui}
  IS-->>MA: 200 {session_token, session, verification_flow_id}
  MA->>MA: SecureTokenStore.write(kratos_session_token) → Authenticated(pendingVerification)
```

## Sequence — verification code delivery (courier)

Recovery codes ([F04](F04-customer-recovery.md)) use the same path.

```mermaid
sequenceDiagram
  autonumber
  participant KP as Kratos courier
  participant WH as identity-service :8081
  participant DB as identity DB
  participant KA as Kratos admin
  participant OB as OpenBao transit
  participant ML as SMTP / SMS

  KP->>WH: POST /internal/hooks/kratos/courier {recipient: handle, template_type, identity_id, code}
  WH->>WH: template must be verification_code_valid or recovery_code_valid (else 204 dropped)
  WH->>DB: reserve courier_dispatch (dedupe_key = HMAC(tt‖recipient‖code))
  alt already reserved
    WH-->>KP: 204 duplicate
  end
  WH->>KA: GET /admin/identities/{identity_id}
  WH->>WH: identity is customer and traits.login_id == recipient (else dropped unbindable)
  WH->>DB: SELECT login_identifier by handle (lazy bind if unbound)
  WH->>OB: transit/decrypt identity-login-kek
  OB-->>WH: email or phone
  WH->>DB: quota check (5/h, 20/day per recipient, SMS daily budget)
  WH->>WH: Render(template, locale from profile, default vi)
  alt email
    WH->>ML: SMTP send
  else phone
    WH->>ML: SMS POST {to, text}
  end
  alt sent
    WH->>DB: MarkSent(state, channel, country)
    WH-->>KP: 204
  else transient error
    WH-->>KP: 503 (reservation released, Kratos retries up to 10x)
  else dropped (provider_rejected, over_quota, unbindable, ...)
    WH->>DB: MarkSent (channel, country NULL)
    WH-->>KP: 204
  end
  Note over WH: a plaintext recipient is accepted only for an admin's own e-mail or a legacy customer in transition (F09)
```

## Errors and outcomes

| Condition | Response | App behaviour |
| --- | --- | --- |
| Missing or too-long password, bad login | 422 `validation_failed` | field error |
| > 5/min or 30/day per IP | 429 `rate_limited`, `Retry-After: 60` | snackbar |
| Address already registered | 400 `auth_flow_rejected` `login`/`4000007` | field error |
| Weak or pwned password | 400 `auth_flow_rejected` `password`/`40000xx` | field error |
| Flow expired | 410 `auth_flow_expired` | restart |
| OpenBao or Kratos down | 503 `dependency_unavailable` | retry |

## Side effects

- `login_identifier`: inserted, then bound by the after-registration hook (or lazily by `GET /v1/me`).
- `profile`: inserted by the after-registration hook.
- `courier_dispatch`: one row per courier message, whether sent or dropped (deduplicated, purged after 24 h).
- Kratos DB: identity with `traits.login_id = handle`.
- **No audit event.** Customer auth flows emit only metrics and logs.

## Code references

- `apps/mobile/lib/features/auth/presentation/sign_up_screen.dart:52`
- `apps/mobile/lib/core/identity/customer_auth_client.dart:61`
- `services/identity-service/internal/app/customerauth.go:228` — `Register`
- `services/identity-service/internal/app/login.go:163` — `claim` / `store`
- `services/identity-service/internal/adapter/kratos/selfservice.go:188` — registration flow
- `services/identity-service/internal/adapter/httpapi/router.go:174` — after-registration / pre-registration hooks
- `services/identity-service/internal/app/login.go:217` — `ValidateRegistration`
- `services/identity-service/internal/app/provisioning.go:25` — profile provisioning
- `services/identity-service/internal/app/courier.go:114` — `Dispatch`
- `deploy/ory/kratos/kratos.yml.tmpl:101` — registration hooks order
- `deploy/ory/kratos/webhooks/pre-registration.jsonnet`, `after-registration.jsonnet`, `courier.jsonnet`

## Related

[ADR-0013](../adr/0013-pseudonymous-customer-login-identifiers.md) ·
[ADR-0014](../adr/0014-customer-login-through-identity-service.md) ·
[ADR-0008](../adr/0008-profile-provisioning.md) ·
[10 §10.2](../architecture/10-pseudonymous-login.md)
