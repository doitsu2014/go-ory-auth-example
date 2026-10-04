# F07 — Customer profile (`/v1/me`)

The customer reads and updates their own profile: display name, avatar,
locale, and their login shown in plain text. The profile row is created
lazily. The login value is decrypted from the login vault on every read, so
**`/v1/me` depends on OpenBao**.

## Actors and entry points

| Actor | Client | Endpoint | Gate |
| --- | --- | --- | --- |
| Customer | `ProfileScreen` | `GET /v1/me` | authenticated customer |
| Customer | `EditProfileScreen` | `PATCH /v1/me {display_name?, avatar_url?, locale?}` | + verified login |

## Functional diagram

```mermaid
flowchart TD
  A["GET or PATCH /v1/me"] --> G["Guard: customer session (F06)"]
  G --> K{"kind = customer?"}
  K -- no --> F403[403 forbidden]
  K -- yes --> M{Method}
  M -- PATCH --> V{"EmailVerified?"}
  V -- no --> NV[403 email_not_verified]
  V -- yes --> PV{"patch.Validate: lengths, characters, http/https avatar, locale"}
  PV -- invalid --> E422[422 validation_failed]
  PV -- ok --> EN
  M -- GET --> EN["ensure profile: SELECT, else INSERT ON CONFLICT DO NOTHING"]
  EN --> U{PATCH?}
  U -- yes --> UP["UPDATE profile ... RETURNING"]
  U -- no --> LG
  UP --> LG["login(): vault row by handle"]
  LG --> LR{"Row found?"}
  LR -- no --> E500["500 internal - login_identifier_missing"]
  LR -- yes --> B{"Bound to this identity?"}
  B -- "unbound / dangling" --> BIND["Lazy bind: UPDATE identity_id, bound_at"]
  B -- "bound to another live identity" --> E500
  B -- yes --> DEC
  BIND --> DEC["OpenBao decrypt identity-login-kek"]
  DEC -- down --> E503[503 dependency_unavailable]
  DEC -- ok --> R["200 Me {id, email_verified, login{type,value}, display_name, avatar_url, locale}"]
```

## Sequence — GET /v1/me

```mermaid
sequenceDiagram
  autonumber
  participant MA as Mobile app
  participant IS as identity-service
  participant DB as identity DB
  participant KA as Kratos admin
  participant OB as OpenBao transit

  MA->>IS: GET /v1/me + Bearer token
  Note over IS: Guard (F06) → Principal{id, LoginID = handle, EmailVerified}
  IS->>DB: SELECT profile WHERE identity_id AND deleted_at IS NULL
  opt not found (lazy provisioning)
    IS->>DB: INSERT profile(identity_id, kind) ON CONFLICT DO NOTHING
    IS->>DB: SELECT profile
  end
  IS->>DB: SELECT login_identifier WHERE pseudonym = handle
  opt identity_id NULL or different
    opt bound to other identity
      IS->>KA: GET /admin/identities/{other}
      Note over IS: rebind only if other is 404, else 500
    end
    IS->>DB: UPDATE login_identifier SET identity_id, bound_at
  end
  IS->>OB: POST transit/decrypt/identity-login-kek (batch, AD kind + handle)
  OB-->>IS: email or phone
  IS-->>MA: 200 {id, email_verified, login:{type,value}, display_name, avatar_url, locale, created_at}
```

## Sequence — PATCH /v1/me

```mermaid
sequenceDiagram
  autonumber
  participant MA as Mobile app
  participant IS as identity-service
  participant DB as identity DB
  participant OB as OpenBao transit

  MA->>IS: PATCH /v1/me {display_name, locale}
  alt not EmailVerified
    IS-->>MA: 403 email_not_verified
    MA->>MA: requireVerification → Verify screen (F02)
  else invalid patch
    IS-->>MA: 422 validation_failed errors[{field, code}]
  else ok
    IS->>DB: ensure profile, UPDATE profile SET display_name, avatar_url, locale, updated_at RETURNING
    IS->>DB: SELECT login_identifier
    IS->>OB: transit/decrypt login
    IS-->>MA: 200 Me
    MA->>MA: invalidate meProvider
  end
```

## Side effects

- `profile`: insert, the first time only, then update on PATCH.
- `login_identifier`: lazy bind.
- **No audit, no Idempotency-Key, no Keto check.** Customer routes are
  `Self` policies.

## Code references

- `apps/mobile/lib/features/profile/data/profile_repository.dart:15`
- `apps/mobile/lib/features/profile/presentation/edit_profile_screen.dart:56`
- `services/identity-service/internal/adapter/httpapi/server.go:49` — `GetMe` / `UpdateMe`, `:388` `toMe`
- `services/identity-service/internal/app/me.go:31` — `GetMe`, `:55` `UpdateMe`, `:80` `ensure`
- `services/identity-service/internal/app/login.go:271` — `bind`, `:319` `Own`
- `services/identity-service/db/queries/profile.sql:1`

## Related

[ADR-0008](../adr/0008-profile-provisioning.md) ·
[10 §10.3](../architecture/10-pseudonymous-login.md)
