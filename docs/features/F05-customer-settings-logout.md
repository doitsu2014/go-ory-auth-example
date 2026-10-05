# F05 — Customer settings (change password) and logout

Changing a password and logging out talk to **Kratos directly** from the
mobile app. identity-service is not involved. The Kratos `profile` settings
method is disabled, so only `password` applies. Profile data is changed
through [F07](F07-customer-profile.md).

## Actors and entry points

| Actor | Client | Entry point |
| --- | --- | --- |
| Customer | `SettingsScreen` | Kratos `GET /self-service/settings/api`, `POST /self-service/settings?flow=` |
| Customer | `SettingsScreen` → Sign out | Kratos `DELETE /self-service/logout/api` |

## Functional diagram

```mermaid
flowchart TD
  A[Change password] --> B["Create settings flow"]
  B --> C["POST {method:password, password}"]
  C --> D{"Kratos result"}
  D -- ok --> E[Snackbar success]
  D -- "policy / HIBP" --> F[Field error]
  D -- expired --> B
  D -- "403 session_refresh_required (session > 15 min)" --> G[Ask current password]
  G --> H["Refresh login: GET /self-service/login/api?refresh=true"]
  H --> I{"Password correct?"}
  I -- no --> F
  I -- yes --> J[Store rotated token if any]
  J --> B
  L[Sign out] --> M["DELETE /self-service/logout/api {session_token} - best effort"]
  M --> N["Clear secure storage → Unauthenticated"]
```

## Sequence — change password

```mermaid
sequenceDiagram
  autonumber
  participant MA as Mobile app
  participant KP as Kratos public
  participant WH as identity-service :8081

  MA->>KP: GET /self-service/settings/api + X-Session-Token
  KP-->>MA: settings flow
  MA->>KP: POST /self-service/settings?flow=id {method:password, password}
  alt success
    KP-->>MA: 200 state success
  else session older than privileged_session_max_age (15m)
    KP-->>MA: 403 session_refresh_required
    MA->>KP: GET /self-service/login/api?refresh=true + X-Session-Token
    MA->>KP: POST /self-service/login?flow=id {method:password, identifier: handle, password}
    KP->>WH: after-login hook (customer + api → 204)
    KP-->>MA: 200 session (token rotated if returned)
    MA->>KP: new settings flow + resubmit new password
  else policy / HIBP failure
    KP-->>MA: 400 flow with errors
  end
```

## Sequence — logout

```mermaid
sequenceDiagram
  autonumber
  participant MA as Mobile app
  participant KP as Kratos public
  participant IS as identity-service
  MA->>KP: DELETE /self-service/logout/api {session_token}
  KP-->>MA: 204 (errors ignored)
  MA->>MA: finally tokens.clear() → Unauthenticated
  Note over IS: SessionCache may still accept the old token for ≤ 30 s
```

## Notes

- The refresh login goes straight to Kratos with the handle. It bypasses
  identity-service's per-IP, per-account and decoy logic. Kratos still runs
  its own checks and the after-login hook.
- No audit events.

## Code references

- `apps/mobile/lib/features/settings/data/settings_repository.dart:22`
- `apps/mobile/lib/features/settings/presentation/settings_screen.dart:35` — change, `:80` refresh, `:166` sign out
- `apps/mobile/lib/features/auth/presentation/auth_controller.dart:84` — `signOut`
- `apps/mobile/lib/features/auth/data/auth_repository.dart:237` — logout
- `deploy/ory/kratos/kratos.yml.tmpl:49` — profile method disabled, `:59` privileged age

## Related

[ADR-0003](../adr/0003-browser-flows-web-native-flows-mobile.md)
