# Research Report

Condition check: runs — the Ory solution space (Kratos vs Hydra, browser vs
native flows, admin/customer separation) has several decisions that change the
architecture.

## Research questions

| # | Question | Decision it unblocks |
| --- | --- | --- |
| RQ1 | Do first-party web + mobile clients need OAuth2/OIDC (Ory Hydra)? | Use Kratos sessions only, or Kratos + Hydra |
| RQ2 | How should a Flutter app authenticate against Kratos? | Native API flows vs browser/WebView |
| RQ3 | How should a React SPA authenticate against Kratos? | Browser flows + cookies vs API flows |
| RQ4 | How to separate admins from customers in one Kratos? | Schemas, selectable flag, roles |
| RQ5 | How does the Go service learn about new registrations? | Webhooks vs lazy provisioning |
| RQ6 | Where do roles/permissions live? | Ory Keto vs identity metadata |

## Findings

**RQ1 — OAuth2 is not needed for first-party apps.** Ory itself argues that
first-party scenarios are simpler with Kratos sessions; OAuth2 forces native
apps through the system browser and complicates session management.
Hydra becomes relevant only for third-party clients, "Sign in with
<our product>", or SSO across independent products.
Source: [Why you probably do not need OAuth2 / OpenID Connect](https://www.ory.com/blog/oauth2-openid-connect-do-you-need-use-cases-examples).
→ *Implication:* v1 uses Kratos sessions; Hydra is a deferred, additive option.

**RQ2 — Mobile uses API (native) flows.** Kratos API flows initialise without
cookies/redirects, return the flow as JSON for native rendering, accept JSON
submission and return a session token. Endpoints: `GET /self-service/{login,registration,recovery,verification,settings}/api`.
Source: [Ory self-service flows](https://www.ory.com/docs/kratos/self-service).
→ *Implication:* Flutter renders Kratos `ui.nodes` natively (or a fixed form),
stores the session token in secure storage, sends it as `X-Session-Token` /
`Authorization: Bearer`.

**RQ3 — SPA must use browser flows.** Ory docs: *"Never use API flows to
implement Browser applications"* — CSRF and login-CSRF vectors. SPAs use
browser flows via AJAX (`Accept: application/json`) with cookies; Kratos public
and the SPA must share a parent domain for cookies.
Source: [Ory self-service flows](https://www.ory.com/docs/kratos/self-service).
→ *Implication:* admin web on `admin.<domain>`, Kratos public on `auth.<domain>`, cookie domain `<domain>`; locally both on `localhost`.

**RQ4 — Multiple identity schemas with `selfservice_selectable`.** Kratos
accepts `?identity_schema=<id>` on registration/login init only for schemas
marked `selfservice_selectable: true`; internal schemas (e.g. admin) stay
non-selectable and are created via the admin API.
Source: [Identity schema selection](https://www.ory.com/docs/identities/model/identity-schema-selection).
→ *Implication:* `customer` = default + selectable; `admin` = non-selectable.
Nobody can self-register as admin.

**RQ5 — Webhooks (`web_hook` action) exist for after-registration/login/
settings/verification.** The `session` action signs a user in right after
registration. Source: [Ory Actions / hooks](https://www.ory.com/docs/kratos/hooks/configure-hooks).
→ *Implication:* after-registration `web_hook` → identity-service creates the
profile; plus idempotent lazy upsert on first API call as a safety net
(webhooks can fail).

**RQ6 — Keto (Zanzibar-style relation tuples)** fits RBAC now and ReBAC later
(e.g. "admin of tenant X"). Identity `metadata_public` is user-read-only but
writable via admin API — workable for a single `role` flag but not for
relationships. *Assumption (team judgement, not a cited source):* Keto's extra
container is worth it because the request explicitly asks for the Ory stack.

## Known failure modes (from experience; treat as assumptions)

- Cookie domain mismatch between SPA and Kratos → login "works" but no session.
- Calling `/sessions/whoami` on every API request without caching → Kratos becomes the bottleneck.
- Exposing Kratos admin port (4434) publicly → full account takeover.
- Reading Kratos tables directly from the app → breaks on Kratos migrations.

## Decisions handed to Ideate

D1 auth model (RQ1), D2 mobile flow (RQ2), D3 admin web flow (RQ3), D4
admin/customer separation (RQ4), D5 provisioning (RQ5), D6 authorization (RQ6),
D7 whether to put Oathkeeper in front of the API.
