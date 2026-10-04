# F02 — Customer verification (email or phone)

After registration the customer proves they own the address by entering a
code. **The mobile app drives the Kratos verification flow directly.**
identity-service is involved only as the courier that delivers the code (see
[F01 courier](F01-customer-registration.md#sequence--verification-code-delivery-courier)).
An unverified customer can sign in and read their data. They cannot change
their profile or personal info (403 `email_not_verified`).

## Actors and entry points

| Actor | Client | Entry point |
| --- | --- | --- |
| Customer | Mobile app `VerifyEmailScreen` | Kratos `GET /self-service/verification/api`, `POST /self-service/verification?flow=` |

## Functional diagram

```mermaid
flowchart TD
  A[Verify screen opens] --> B{"verification_flow_id from registration?"}
  B -- yes --> R["Reuse flow, code already sent"]
  B -- no --> C["Create native verification flow"]
  C --> D["POST {method:code, email: own handle from stored session}"]
  D --> R
  R --> E[Customer enters code]
  E --> F{"Kratos checks code"}
  F -- wrong --> G[Flow errors shown, can resend]
  G --> E
  F -- expired --> H["FlowExpiredFailure → new flow"]
  H --> C
  F -- passed_challenge --> I["GET /sessions/whoami → refresh session"]
  I --> J["Authenticated, loginVerified = true → Profile"]
  A -.-> S["Skip → continue unverified"]
  S --> U{"Later PATCH /v1/me or PUT personal-info"}
  U --> V["403 email_not_verified → requireVerification → back here"]
```

## Sequence

```mermaid
sequenceDiagram
  autonumber
  participant MA as Mobile app
  participant KP as Kratos public
  participant IS as identity-service :8081 (courier)
  participant ML as SMTP / SMS

  alt flow id from registration
    Note over MA: reuse verification_flow_id (code already queued at registration)
  else no flow id or resend
    MA->>KP: GET /self-service/verification/api
    KP-->>MA: flow
    MA->>KP: POST /self-service/verification?flow=id {method:code, email: handle}
    Note right of MA: handle = session.identity.traits.login_id from secure storage
  end
  KP-)IS: courier verification_code_valid {recipient: handle, code}
  IS->>ML: decrypt address, send code (see F01 courier)
  MA->>KP: POST /self-service/verification?flow=id {method:code, code}
  alt state = passed_challenge
    KP-->>MA: 200 flow passed_challenge
    MA->>KP: GET /sessions/whoami + X-Session-Token
    KP-->>MA: session (verifiable_address for login_id verified)
    MA->>MA: Authenticated(session) → Profile
  else wrong code
    KP-->>MA: 200 flow with ui messages → FlowValidationFailure
  else expired
    KP-->>MA: 410 self_service_flow_expired → FlowExpiredFailure
  end
```

## How "verified" is computed

The app and the server use the same rule. A customer is verified when the
Kratos `verifiable_addresses` entry whose value equals `traits.login_id` is
verified. On the server this sets `Principal.EmailVerified`; see
[F06](F06-request-authentication.md).

## Code references

- `apps/mobile/lib/features/auth/presentation/verify_email_screen.dart:40`
- `apps/mobile/lib/features/auth/data/auth_repository.dart:129` — start / resend / submit
- `apps/mobile/lib/core/kratos/ory_kratos_client.dart:121`
- `apps/mobile/lib/core/kratos/kratos_models.dart:245` — `loginId`, `loginVerified`
- `services/identity-service/internal/adapter/kratos/model.go:66` — server-side `EmailVerified`
- `services/identity-service/internal/app/me.go:57` — `email_not_verified` gate
- `deploy/ory/kratos/kratos.yml.tmpl:72` — verification `use: code`

## Related

[ADR-0003](../adr/0003-browser-flows-web-native-flows-mobile.md) ·
[03 §3.1](../architecture/03-auth-flows.md)
