# 3. Authentication & Authorization Flows

> **Current diagrams:** some sequences on this page predate ADR-0014 and the AdminGate MFA rules. The per-feature pages, checked against the code, are authoritative: [F01](../features/F01-customer-registration.md) · [F02](../features/F02-customer-verification.md) · [F03](../features/F03-customer-login.md) · [F04](../features/F04-customer-recovery.md) · [F05](../features/F05-customer-settings-logout.md) · [F06](../features/F06-request-authentication.md) · [F09](../features/F09-admin-sign-in.md) · [F10](../features/F10-admin-management.md).

Ground rule ([ADR-0003](../adr/0003-browser-flows-web-native-flows-mobile.md)):

| Client | Kratos flow type | Credential carried to our API |
| --- | --- | --- |
| Admin Web (browser) | **Browser flows** (`/self-service/*/browser`, AJAX with `Accept: application/json`, `credentials: "include"`) | Cookie `ory_kratos_session` (HttpOnly, Secure, SameSite=Lax, non-persistent, domain = dedicated platform apex, see [05-deployment](05-deployment.md#52-production-topology)) — **only** credential accepted on `/admin/v1/*` |
| Mobile (native) | **API flows** (`/self-service/*/api`) | `Authorization: Bearer <session_token>` — **only** credential accepted on `/v1/*` |

Credentials are **bound to their plane**: identity-service rejects a bearer
token on `/admin/v1/*` and a cookie on `/v1/*` (`401`). This stops an admin from
obtaining a long-lived native-flow token and using it against the admin API,
bypassing browser/CSRF controls.

> Ory docs: *never use API flows to implement browser applications* — they
> remove CSRF protection. Conversely, never embed browser flows in a mobile WebView.

Admin passwords go **directly from the browser to Kratos public**. Customer
sign-in and registration go through identity-service (ADR-0014). The app
sends the email or phone number and the password to
`POST /v1/auth/{login,registration}`. identity-service finds the account's
opaque handle `<base32>@login.invalid` and runs the Kratos native API flow
server side, holding the password in memory only. Kratos never sees the
address (ADR-0013), and the app never sees a handle before it is signed in.

Steps that need no address stay direct to Kratos:

- verification (with the owner's own handle);
- the settings flow (the new password after recovery; the recovery code
  itself goes through `POST /v1/auth/recovery/code`);
- whoami;
- logout.

Kratos sends codes through its `http` courier to identity-service, which
delivers them to the real address (§3.11).

---

## 3.1 Customer registration (mobile) + email verification

```mermaid
sequenceDiagram
  autonumber
  actor U as Customer
  participant A as Mobile App
  participant K as Kratos public
  participant S as identity-service (internal :8081)
  participant M as SMTP

  U->>A: Tap "Sign up" → email or phone, password
  A->>S: POST /v1/auth/registration {login:{type, value}, password}
  S->>S: HMAC lookup key → vault row, or seal the address under a new random handle
  S->>K: GET /self-service/registration/api → POST {method:"password", traits:{login_id: handle}, password}
  K->>S: pre-registration webhook (parse:true): vault entry exists?
  alt validation error (weak/leaked password, identifier taken)
    K-->>S: 400 flow with ui.messages
    S-->>A: 400 auth_flow_rejected {errors:[{field, code: Kratos id}]} → show errors, retry
  else success
    K->>S: POST /internal/hooks/kratos/after-registration (async, ignored response)
    S->>S: upsert profile(identity_id), bind login vault entry
    K->>S: courier http {recipient: handle, code}
    S->>M: verification code to the real email (or SMS)
    K-->>S: 200 {session, session_token}
    S-->>A: 200 {session_token, session, verification_flow_id?}
    A->>A: store session_token in secure storage
  end
  Note over A,K: Native registration does not return show_verification_ui<br/>(observed on Kratos v26.2.0) — the app starts its own flow
  A->>K: GET /self-service/verification/api → POST {method:"code", email: own handle (whoami)}
  K->>S: courier http {recipient: handle, code}
  S->>M: fresh verification code (email or SMS)
  A->>U: "Enter the code we emailed you"
  U->>A: 6-digit code
  A->>K: POST /self-service/verification?flow={vid} {method:"code", code}
  K-->>A: 200 flow state "passed_challenge"
```

Notes
- Registration uses the default schema `customer`; the app may pass
  `?identity_schema=customer` explicitly. The `admin` schema is **not
  selectable**, so self-registering as admin is impossible.
- The `session` hook signs the user in immediately. Endpoints that need a
  verified address return `403 email_not_verified` until verification passes.
- If the webhook fails, the profile is created lazily on the first API call
  ([ADR-0008](../adr/0008-profile-provisioning.md)).

## 3.2 Customer login (mobile) and calling the API

```mermaid
sequenceDiagram
  autonumber
  participant A as Mobile App
  participant K as Kratos public
  participant S as identity-service
  participant C as Session cache
  participant DB as PostgreSQL (identity)

  A->>S: POST /v1/auth/login {login:{type, value}, password}
  S->>S: HMAC lookup key → handle (random decoy if unknown)
  S->>K: GET /self-service/login/api → POST {method:"password", identifier: handle, password}
  K-->>S: 200 {session, session_token} | 400 flow
  S-->>A: 200 {session_token, session} | 400 auth_flow_rejected (ids only)
  A->>A: save token (Keychain / Keystore)

  A->>S: GET /v1/me  Authorization: Bearer <token>
  S->>C: get(sha256(token))
  alt cache miss
    S->>K: GET /sessions/whoami  X-Session-Token: <token>
    K-->>S: 200 session {identity{id, schema_id, traits, state}, aal, expires_at}
    S->>C: put(ttl = min(30s, expires_at - now))
  end
  S->>S: require schema_id == "customer", active, not expired
  S->>DB: SELECT profile WHERE identity_id = $1 (upsert if missing)
  S-->>A: 200 {id, email, display_name, ...}
```

Token lifecycle: Kratos sessions are long-lived for mobile (`session.lifespan`,
e.g. 30 days). `/sessions/whoami` does **not** extend a session. Extension
happens by a refresh login (`/self-service/login/api?refresh=true`) or by
identity-service calling Kratos admin `PATCH /admin/sessions/{id}/extend`, both
gated by `session.earliest_possible_extend`. v1: the app re-authenticates when
the session expires. On `401` the app clears the token and returns to sign-in.

## 3.3 Password recovery (mobile)

```mermaid
sequenceDiagram
  autonumber
  participant A as Mobile App
  participant S as identity-service
  participant K as Kratos public
  participant M as SMTP
  A->>S: POST /v1/auth/recovery {login:{type, value}}
  S->>K: GET /self-service/recovery/api → POST {method:"code", email: handle or decoy}
  S-->>A: 200 {recovery_id} (flow id sealed with the login KEK, identical shape whether or not the account exists)
  K->>M: recovery code via identity-service courier (sent only if the account exists)
  A->>S: POST /v1/auth/recovery/code {recovery_id, code}
  S->>K: POST /self-service/recovery?flow={id} {method:"code", code}
  K-->>S: 200 continue_with [set_ory_session_token, show_settings_ui]<br/>(requires feature_flags.use_continue_with_transitions: true)
  S-->>A: 200 {session_token, settings_flow_id} | 400 auth_flow_rejected (4060006) | 410 auth_flow_expired
  A->>K: POST /self-service/settings?flow={sid} {method:"password", password}<br/>X-Session-Token
  K-->>A: 200 settings saved
```

The Kratos flow id never reaches the app in clear. `GET
/self-service/recovery/flows?id=` is public and its `email` node shows the
handle (ADR-0014, found in review).

Recovery is configured to revoke other sessions (`revoke_active_sessions`
after recovery).

## 3.4 Admin login (browser) with mandatory MFA

```mermaid
sequenceDiagram
  autonumber
  actor Ad as Admin
  participant W as Admin Web (admin.<domain>)
  participant K as Kratos public (auth.<domain>)
  participant S as identity-service (api.<domain>)

  Ad->>W: open /customers
  W->>S: GET /admin/v1/me (cookie)
  S-->>W: 401
  W->>K: GET /self-service/login/browser (Accept: json, credentials: include)
  K-->>W: 200 flow + csrf cookie
  Ad->>W: email + password
  W->>K: POST /self-service/login?flow={id} {method:"password", identifier, password, csrf_token}
  K-->>W: 200 + Set-Cookie ory_kratos_session (aal1)
  W->>S: GET /admin/v1/me
  S->>K: GET /sessions/whoami (Cookie)
  K-->>S: 403 session_aal2_required (TOTP enrolled, session is aal1)
  S-->>W: 403 {code:"aal2_required"}
  W->>K: GET /self-service/login/browser?aal=aal2
  Ad->>W: TOTP code
  W->>K: POST /self-service/login?flow={id2} {method:"totp", totp_code, csrf_token}
  K-->>W: 200 + Set-Cookie (aal2)
  W->>S: GET /admin/v1/me
  S->>S: schema_id=="admin" ∧ aal=="aal2" ∧ Keto check
  S-->>W: 200 {id, email, roles, permissions}
```

- Kratos is configured with `session.whoami.required_aal: highest_available`
  (set explicitly, it is also the default) and
  `selfservice.flows.settings.required_aal: highest_available`. whoami therefore
  answers **`403 session_aal2_required`** (not 200, not 401) for an AAL1 session
  of an identity that has TOTP. The verifier maps that to our `aal2_required`;
  it must never treat it as `401`, or the admin loops on the login page.
- **TOTP enrolment happens inside the invitation**: the invite recovery code
  opens a privileged settings flow where the new admin sets a password **and**
  enrols TOTP in the same session (§3.6). An admin identity with no TOTP gets
  `mfa_enrollment_required` only within 24 h of the invite. After that the
  service disables the identity, so someone with a stolen password can't enrol
  their own second factor. `/admin/v1/me` is the only admin endpoint reachable
  at AAL1.
- **Wrong population on the wrong client is blocked at Kratos**: an
  interrupting after-login `web_hook` (`can_interrupt: true`) rejects
  `customer` identities on browser flows and `admin` identities on API flows
  ([ADR-0004](../adr/0004-single-kratos-two-identity-schemas.md); spike S3
  verifies support in the pinned Kratos release). If it's unavailable, the
  fallback is that every `/admin/*` call returns `403 not_admin` and the web
  logs the session out.
- Admin sessions are capped at **12 h since `authenticated_at`**. Once the cap
  is passed, the service revokes the session through Kratos admin
  `DELETE /admin/sessions/{id}` and doesn't stop at returning 403. The admin
  cookie is non-persistent (`session.cookie.persistent: false`).

## 3.5 Request authorization pipeline (identity-service)

```mermaid
flowchart LR
  req[HTTP request] --> rid[Request ID + logging]
  rid --> authn{Authenticate<br/>/v1: bearer only<br/>/admin/v1: cookie only}
  authn -- none/invalid --> e401[401 unauthenticated]
  authn -- session --> plane{Route plane}
  plane -- /v1/* --> cust{schema == customer?}
  plane -- /admin/v1/* --> adm{schema == admin?}
  cust -- no --> e403[403 forbidden]
  adm -- no --> e403
  adm -- yes --> aal{AAL2 and<br/>age < 12h?}
  aal -- no --> e403a[403 aal2_required]
  aal -- yes --> keto{Keto check<br/>Console:main#permission}
  keto -- denied --> e403
  keto -- allowed --> uc[Use case]
  cust -- yes --> uc
  uc --> audit[(Audit if mutation)]
```

Permission per endpoint is declared next to the route (see
[API contract](../api/identity-service-api.md)); routes without a declared
permission fail closed at startup.

## 3.6 Admin invites another admin

```mermaid
sequenceDiagram
  autonumber
  participant W as Admin Web
  participant S as identity-service
  participant KA as Kratos admin (private)
  participant KT as Keto write (private)
  participant DB as identity DB
  participant M as SMTP
  W->>S: POST /admin/v1/admins {email, name, role} + Idempotency-Key
  S->>S: require Console:main#manage_admins; role ∈ {support, admin, super_admin}
  S->>KA: POST /admin/identities {schema_id:"admin", traits, state:"active"}
  S->>KT: PUT relation-tuples Console:main#{relation(role)}@User:{id}
  S->>KA: POST /admin/recovery/code {identity_id, expires_in:"24h"}
  KA-->>S: {recovery_link, recovery_code}  (Kratos does NOT email these)
  S->>M: invitation email (identity-service mailer)
  S->>DB: INSERT audit(admin.invited) + profile
  S-->>W: 201 {id, email, role, invitation_expires_at}
```

- Kratos returns the admin-created recovery link and code in the API response
  and does not send them. identity-service sends the invitation through its own
  mailer. The link never appears in an API response or a log.
- `role` is a closed enum mapped server-side to a relation
  (`support → supporters`, `admin → admins`, `super_admin → super_admins`).
  Client input never forms a tuple directly. The service forbids changing your
  own role and removing the last `super_admin`.
- New admin path: open link, set password, enrol TOTP in the same privileged
  session, then the first AAL2 login.

The first `super_admin` is created by `identity-service admin bootstrap`
(same steps, run once from a trusted shell; it prints the link to the
operator's terminal only).

## 3.7 Admin disables a customer

1. `POST /admin/v1/customers/{id}/disable` (permission `manage_customers`).
   The target must have `schema_id == customer`; otherwise `404`. The same
   check applies to every `/customers/{id}/*` endpoint.
2. Kratos admin `PATCH /admin/identities/{id}` → `state: inactive`.
3. Kratos admin `DELETE /admin/identities/{id}/sessions` (revoke all).
4. Purge local session cache for that identity; audit `customer.disabled`.
5. Residual risk: other replicas may accept the old token for ≤ cache TTL (30 s).

## 3.8 Logout

| Client | Steps |
| --- | --- |
| Mobile | `DELETE /self-service/logout/api` body `{session_token}` → delete token from secure storage |
| Admin Web | `GET /self-service/logout/browser` → `{logout_url}` → navigate/fetch `logout_url` → cookie cleared |

## 3.9 Handling Kratos flow responses (both clients)

| Response | Meaning | Client action |
| --- | --- | --- |
| `200` | Success | Continue (`continue_with` may request verification / redirect) |
| `400` + flow | Validation error | Re-render the **same** flow with `ui.messages` / node messages |
| `401` | No session (settings flow) | Go to login |
| `403` `session_aal2_required` | Need second factor | Start login flow with `aal=aal2` |
| `403` `security_csrf_violation` (web) | Stale CSRF | Restart flow |
| `410` | Flow expired | Start a new flow |
| `422` `browser_location_change_required` | Redirect required (e.g. OIDC later) | Follow `redirect_browser_to` |

Always render messages by their `id` (stable Kratos message ids) so the UI can
be localised (Vietnamese / English) without parsing English text.

## 3.10 Future: OAuth2 / OIDC with Ory Hydra

When a third-party client or SSO across products is needed, add Hydra with
Kratos as its login & consent provider. Existing clients keep using Kratos
sessions; only new clients use OAuth2, and identity-service learns to accept
Hydra JWT access tokens in addition to sessions
([ADR-0002](../adr/0002-kratos-sessions-over-oauth2.md)).

## 3.11 Pseudonymous customer login identifiers (ADR-0013, ADR-0014)

Full diagrams per flow, the API calls between services, pros/cons and
future directions: [10-pseudonymous-login](10-pseudonymous-login.md).

| Step | Who | What |
| --- | --- | --- |
| Look up | App → identity-service `POST /v1/auth/{login,registration,recovery}` (+ `/recovery/code`) | Normalise (email lower-cased, phone E.164 with the default country `84`), compute the HMAC lookup key in OpenBao, read the vault row. Registration stores the address (encrypted) under a new random handle. No handle is ever returned to an unauthenticated caller. Rate limited per IP, per network and per account (every sign-in attempt). |
| Register | identity-service → Kratos (API flow) | `traits.login_id = handle`. The pre-registration webhook (`response.parse: true`) rejects a handle without a vault entry (4049002), a legacy `email` trait (4049001) and, during the migration window, an address that a legacy customer already uses (4000007). |
| Bind | Kratos → after-registration webhook (or lazily on `GET /v1/me`) | The vault entry is bound to the identity id. The binding uses only the authoritative `login_id` from Kratos. |
| Deliver | Kratos courier `http` → `POST /internal/hooks/kratos/courier` (courier key) | Checks that the payload's identity really has this recipient, decrypts the address, renders the vi/en message and sends email or SMS. Duplicates are dropped (keyed hash), as are messages over quota (5/h and 20/day per recipient, SMS budget). Permanent failures get 204, transient ones 503, which makes Kratos retry. |
| Show | `GET /v1/me` / admin views | The owner sees `login {type, value}`. Admins see `login {type, masked}`. A reveal (`fields: ["login"]`) is audited. |

The Kratos `profile` settings method is disabled, so a customer cannot repoint
`login_id`. Admin identities keep their plaintext work email; their Kratos
mail also goes through the courier webhook, which sends it only to the
admin's own address.
