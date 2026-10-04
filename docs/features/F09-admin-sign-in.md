# F09 — Admin sign-in, MFA, recovery, settings and logout

Admins use the React console. The SPA drives Kratos **browser flows**
directly, using the `ory_kratos_session` cookie and CSRF tokens. It then asks
identity-service `GET /admin/v1/me` whether this admin may enter. Admins are
invite-only ([F10](F10-admin-management.md)). TOTP is required within 24 h of
the invitation. The `AdminGate` re-checks this on every request and caps
session age at 12 h.

## Actors and entry points

| Actor | SPA route | Calls |
| --- | --- | --- |
| Admin | `/login` | Kratos `GET /self-service/login/browser`, `POST /self-service/login?flow=` |
| Admin | console routes (`requireAdminLoader`) | identity-service `GET /admin/v1/me` |
| Admin | `/settings` (no guard) | Kratos settings flow: password, TOTP, lookup secrets |
| Admin | `/recovery`, `/verification` | Kratos recovery/verification flows (code) |
| Admin | Sign out | Kratos `GET /self-service/logout/browser`, `GET /self-service/logout?token=` |
| System | every 5 min | `AdminGate.Sweep` |

## Functional diagram — console guard

```mermaid
flowchart TD
  A[Open console route] --> B["requireAdminLoader → GET /admin/v1/me"]
  B --> C{"Cookie present, no bearer?"}
  C -- no --> L401
  C -- yes --> W{"Kratos whoami"}
  W -- "401 / inactive" --> L401["SPA → /login?return_to=path"]
  W -- "403 session_aal2_required" --> AAL2["SPA → /login?aal=aal2"]
  W -- "5xx" --> E503[RouteError 503]
  W -- 200 --> K{"schema = admin?"}
  K -- no --> NA["403 not_admin → /no-access"]
  K -- yes --> AGE{"authenticated_at older than 12 h?"}
  AGE -- yes --> REV["Kratos DELETE /admin/sessions/{sid}"] --> L401
  AGE -- no --> ID["Kratos admin GET identity ?include_credential=totp"]
  ID --> ST{"state active?"}
  ST -- no --> L401
  ST -- yes --> T{"TOTP enrolled?"}
  T -- "no, before deadline" --> ENR["403 mfa_enrollment_required → /settings?enroll=totp"]
  T -- "no, after deadline" --> DEA["deactivate + audit admin.deactivated_mfa_deadline → 403 → /no-access"]
  T -- "yes, enrolled after deadline" --> DEA
  T -- yes --> A2{"AAL2, or route AllowAAL1?"}
  A2 -- no --> AAL2
  A2 -- yes --> ME["Keto: roles + 6 permission checks → 200 AdminMe"]
  ME --> NAV["ConsoleLayout: nav filtered by permissions"]
```

## Sequence — password + TOTP sign-in

```mermaid
sequenceDiagram
  autonumber
  participant AW as Admin web (SPA)
  participant KP as Kratos public :4433
  participant WH as identity-service :8081
  participant IS as identity-service :8080
  participant KA as Kratos admin
  participant KR as Keto read

  AW->>KP: GET /self-service/login/browser (Accept JSON, credentials include)
  KP-->>AW: flow + csrf cookie
  AW->>KP: POST /self-service/login?flow=id {method:password, identifier, password, csrf_token}
  KP->>WH: POST /internal/hooks/kratos/after-login {identity_id, schema_id, flow_type}
  alt admin and flow_type = browser
    WH-->>KP: 204
  else otherwise
    WH-->>KP: 403 message 4000001 → flow re-rendered with error
  end
  KP-->>AW: 422 browser_location_change_required (AAL2 needed) + Set-Cookie aal1
  AW->>KP: GET /self-service/login/browser?aal=aal2
  AW->>KP: POST /self-service/login?flow=id {method:totp, totp_code, csrf_token}
  KP-->>AW: 200 + Set-Cookie ory_kratos_session (aal2)
  AW->>AW: removeQueries(me), navigate return_to or /customers
  AW->>IS: GET /admin/v1/me (Cookie)
  IS->>KP: GET /sessions/whoami (Cookie only)
  KP-->>IS: session aal2, identity schema admin
  IS->>KA: GET /admin/identities/{id}?include_credential=totp
  Note over IS: AdminGate: age ≤ 12 h, active, TOTP before deadline, AAL2
  IS->>KR: GET /relation-tuples?namespace=Console&object=main&subject_set=User:id
  loop 6 permissions
    IS->>KR: POST /relation-tuples/check/openapi Console:main#perm@User:id
  end
  IS-->>AW: 200 {id, email, name, aal, roles, permissions, session_expires_at}
```

## Sequence — first-time enrolment (from invitation)

```mermaid
sequenceDiagram
  autonumber
  participant AD as New admin (browser)
  participant AW as Admin web
  participant KP as Kratos public
  participant WH as identity-service :8081
  participant ML as Mailpit / SMTP

  AD->>KP: open recovery_link from invite e-mail (F10)
  KP-->>AW: 303 → /recovery?flow=id
  AW->>KP: POST /self-service/recovery?flow=id {method:code, code, csrf_token}
  KP-->>AW: privileged aal1 session, continue_with show_settings_ui
  AW->>AW: navigate /settings?flow=sid
  AW->>KP: POST /self-service/settings?flow=sid {method:password, password}
  AW->>KP: POST /self-service/settings?flow=sid {method:totp, totp_code}
  KP-->>AW: 200 settings saved (TOTP enrolled)
  Note over AW: next console visit → /login?aal=aal2 → TOTP → /admin/v1/me 200
  opt forgot password later
    AW->>KP: POST /self-service/recovery {method:code, email}
    KP->>WH: courier recovery_code_valid {recipient: admin e-mail}
    WH->>ML: send (plaintext allowed only for the admin's own e-mail)
  end
```

## Sequence — logout and MFA sweeper

```mermaid
sequenceDiagram
  autonumber
  participant AW as Admin web
  participant KP as Kratos public
  participant IS as identity-service
  participant KA as Kratos admin
  participant DB as identity DB

  AW->>KP: GET /self-service/logout/browser
  KP-->>AW: {logout_token, logout_url}
  AW->>KP: GET /self-service/logout?token=logout_token (fetch)
  KP-->>AW: 204 cookie cleared (401 also treated as success)
  AW->>AW: queryClient.clear(), navigate /login

  loop every ADMIN_MFA_SWEEP_INTERVAL (5 min)
    IS->>KA: GET /admin/identities?page_size=100&include_credential=totp
    IS->>IS: admins active, no TOTP past deadline (or TOTP enrolled late)
    rect rgba(128,128,128,0.12)
      IS->>DB: INSERT audit_event admin.deactivated_mfa_deadline (system actor)
      IS->>KA: PATCH /admin/identities/{id} state=inactive
      IS->>KA: DELETE /admin/identities/{id}/sessions
      IS->>DB: COMMIT
    end
  end
```

## SPA error routing

| Response | SPA route |
| --- | --- |
| 401 | `/login?return_to=…` |
| 403 `aal2_required` | `/login?aal=aal2` |
| 403 `mfa_enrollment_required` | `/settings?enroll=totp` |
| 403 `not_admin` / other 403 | `/no-access` (manual sign-out) |
| Kratos `session_refresh_required` | `/login?refresh=true` |
| Kratos CSRF violation / 404 / 410 | restart the flow (at most 2 times) |

## Code references

- `apps/admin-web/src/app/routes.tsx:30` — route tree
- `apps/admin-web/src/auth/requireAdmin.ts:15` — `authRedirectFor`, loader
- `apps/admin-web/src/auth/useKratosFlow.ts:64` — flow init and error mapping
- `apps/admin-web/src/features/login/LoginPage.tsx:42`
- `apps/admin-web/src/features/settings/SettingsPage.tsx:35`
- `apps/admin-web/src/features/recovery/RecoveryPage.tsx:25`
- `apps/admin-web/src/auth/useLogout.ts:20`
- `services/identity-service/internal/app/admingate.go:49` — `Check`, `:111` `Sweep`, `:139` `deactivate`
- `services/identity-service/internal/app/admins.go:81` — `Me`
- `services/identity-service/internal/adapter/httpapi/middleware.go:330` — CSRF / Origin
- `deploy/ory/kratos/identity-schemas/admin.v1.json`

## Related

[ADR-0003](../adr/0003-browser-flows-web-native-flows-mobile.md) ·
[ADR-0004](../adr/0004-single-kratos-two-identity-schemas.md) ·
[03 §3.4–3.5](../architecture/03-auth-flows.md)
