# 2. Components

Each component lists what it **owns**, what it **exposes**, what it **depends
on**, and what it must **never** do.

## 2.1 Ory Kratos — identity & authentication

| | |
| --- | --- |
| **Owns** | Identities, traits (customers: only the pseudonymous `login_id`, ADR-0013), credentials (password hashes, TOTP secrets, WebAuthn keys, recovery codes), sessions, self-service flows, the courier queue (delivery is done by identity-service over `http`) |
| **Exposes** | Public API `:4433` (self-service flows, `/sessions/whoami`, logout) — internet-facing via `auth.<domain>`. Admin API `:4434` (identity CRUD, session revoke, recovery links) — **private only** |
| **Depends on** | PostgreSQL db `kratos`, identity-service webhook endpoints (pre/after-registration, after-login, courier) |
| **Config** | `deploy/ory/kratos/kratos.yml.tmpl` (rendered at deploy time), identity schemas in `deploy/ory/kratos/identity-schemas/`, webhook body templates (Jsonnet) |
| **Never** | Be extended by forking; be read via SQL by other components; expose `:4434` publicly |

Enabled self-service methods (v1): `password`, `code` (verification &
recovery only, with `passwordless_enabled: false` and `mfa_enabled: false`
pinned so that email OTP can never count as a login factor), `totp`,
`lookup_secret` (backup codes). Later: `webauthn`/`passkey`, `oidc`. The
`profile` settings method is **disabled**, so traits never change through
self-service (ADR-0013: a customer cannot repoint `login_id`).

## 2.2 Ory Keto — authorization

| | |
| --- | --- |
| **Owns** | Relation tuples (who has which role / relationship) and the permission model (Ory Permission Language, OPL) |
| **Exposes** | Read API `:4466` (check, expand, list) and Write API `:4467` — **both private** |
| **Depends on** | PostgreSQL db `keto` |
| **Never** | Be called by clients directly; hold business data |

Permission model v1 (`deploy/ory/keto/namespaces.keto.ts`):

```ts
import { Namespace, Context } from "@ory/keto-namespace-types"

class User implements Namespace {}

class Role implements Namespace {
  related: { members: User[] }
}

// A single object "console" represents the admin plane.
class Console implements Namespace {
  related: {
    super_admins: (User | SubjectSet<Role, "members">)[]
    admins:       (User | SubjectSet<Role, "members">)[]
    supporters:   (User | SubjectSet<Role, "members">)[]
  }
  permits = {
    manage_admins: (ctx: Context) => this.related.super_admins.includes(ctx.subject),
    manage_customers: (ctx: Context) =>
      this.related.super_admins.includes(ctx.subject) ||
      this.related.admins.includes(ctx.subject),
    view_customers: (ctx: Context) =>
      this.permits.manage_customers(ctx) ||
      this.related.supporters.includes(ctx.subject),
    view_audit: (ctx: Context) => this.permits.manage_customers(ctx),
  }
}
```

Example tuples: `Console:main#admins@User:7f3c…`, `Console:main#supporters@User:a19e…`.

## 2.3 identity-service — Go

The application backend. "Identity service" here means **our domain service
around identity**, not a replacement for Kratos.

| | |
| --- | --- |
| **Owns** | `identity` database: customer/admin **profiles** (app-specific data keyed by Kratos identity id), **audit log** of admin actions |
| **Exposes** | `:8080` public REST `/v1/*` (customers, bearer only; `POST /v1/auth/identifiers` is public and rate limited per IP) and `/admin/v1/*` (admins, cookie only) on `api.<domain>`; `:8081` **webhook-only** `/internal/hooks/kratos/*` (reachable only by Kratos); `:9090` ops `/healthz`, `/readyz`, `/metrics` (probes + Prometheus only) |
| **Depends on** | Kratos public (session check), Kratos admin (identity management, recovery codes, session revoke), Keto read/write, PostgreSQL db `identity`, OpenBao Transit, SMTP (invitations and every Kratos message), SMS provider (phone logins) |
| **Never** | Receive or store passwords; proxy Kratos self-service flows; read Kratos/Keto tables; trust identity data sent by clients |

Responsibilities:

1. **Authenticate** every request: resolve the plane's credential (bearer token
   on `/v1`, cookie on `/admin/v1`, never both) to a Kratos session (cached
   ≤ 30 s), reject inactive / expired sessions.
2. **Authorize** per use case: customer endpoints need schema `customer`;
   admin endpoints need schema `admin` + AAL2 + Keto permission.
3. **Profiles**: `GET/PATCH /v1/me`; created by the after-registration webhook or
   lazily on first request (idempotent).
4. **Admin use cases**: list/search/view customers (via Kratos admin API),
   disable/enable (Kratos `state`), revoke sessions, invite admins, assign roles
   (Keto), read audit log. Invitation emails are sent by identity-service's
   own mailer, because Kratos doesn't email admin-created recovery codes.
5. **Audit**: append-only record of every admin mutation.
6. **Bootstrap CLI**: `identity-service admin bootstrap --email …` creates the
   first `super_admin` and prints a one-time recovery link.

Internal structure: see [Go backend guidelines](../principles/02-backend-go.md).

## 2.4 Admin Web — React SPA

| | |
| --- | --- |
| **Owns** | Admin UI only; no server-side state |
| **Talks to** | Kratos public (browser flows, `credentials: "include"`), identity-service `/admin/v1/*` (cookie) |
| **Pages** | `/login`, `/login?aal=aal2` (MFA step-up), `/settings` (password, TOTP, backup codes), `/recovery`, `/customers`, `/customers/:id`, `/admins`, `/audit`, `/error` |
| **Never** | Offer registration; store tokens in `localStorage`; use Kratos API (native) flows; decide authorization by itself (UI hides, API enforces) |

Details: [Admin web guidelines](../principles/03-admin-web-react.md).

## 2.5 Mobile App — Flutter

| | |
| --- | --- |
| **Owns** | Customer UI; session token in secure storage |
| **Talks to** | Kratos public (native API flows: registration, login, verification, recovery, settings, logout), identity-service `/v1/*` (`Authorization: Bearer <session_token>`) |
| **Screens** | Welcome, Sign up, Verify email (code), Sign in, Forgot password (code), Profile, Security settings, Sign out |
| **Never** | Use browser flows or a WebView for Kratos; log or persist the token outside secure storage; embed any secret |

Details: [Mobile guidelines](../principles/04-mobile-flutter.md).

## 2.6 PostgreSQL

One cluster (v16+), three databases, three login roles. See
[Data architecture](04-data.md).

## 2.7 Supporting infrastructure

| Component | Purpose | Local | Production |
| --- | --- | --- | --- |
| Reverse proxy | TLS, routing by host, rate limiting | none (direct ports) or Caddy | Ingress / Caddy / Traefik / cloud LB |
| Mailpit | Catch emails locally | `:8025` UI, `:1025` SMTP | replaced by SMTP provider (SES, Postmark, …) |
| OpenTelemetry Collector | Traces/metrics pipeline | optional | yes |

## 2.8 Responsibility matrix

| Concern | Kratos | Keto | identity-service | Admin Web | Mobile |
| --- | --- | --- | --- | --- | --- |
| Password / MFA / recovery | **R** | | | UI | UI |
| Session issue & revoke | **R** | | triggers revoke (admin) | logout | logout |
| Session validation for our API | provides | | **R** | | |
| Roles & permissions | | **R** (data) | **R** (enforce) | hides UI | |
| Profile data | | | **R** | | |
| Customer login pseudonym (trait `login_id`) | **R** | | resolves, binds, migrates | | submits to Kratos flows |
| Customer email / phone (login vault, encrypted) | | | **R** | masked / reveal | resolves before each flow |
| Audit of admin actions | | | **R** | | |

R = responsible / source of truth.
