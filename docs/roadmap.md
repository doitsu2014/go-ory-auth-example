# Roadmap (work breakdown seeds)

Build the **walking skeleton** first (P13), then widen. Each unit is one PR,
linked to its requirement ids.

## Milestone 0 — Spikes (≤ 1 day)

| Unit | Goal | Req |
| --- | --- | --- |
| S1 | Flutter: native login/registration against local Kratos; compare hand-built vs generic `ui.nodes` rendering | FR-01, FR-02 |
| S2 | React SPA on `localhost:5173` + Kratos `:4433` browser login with cookies/CORS | FR-05 |
| S3 | Interrupting after-login webhook that blocks the wrong population per flow type (required by ADR-0004; verify support in the pinned Kratos) + confirm whoami `403 session_aal2_required` behaviour | FR-06 |

## Milestone 1 — Walking skeleton

| # | Unit | Depends on | Req |
| --- | --- | --- | --- |
| U1 | `deploy/`: compose with Postgres (3 DBs), Kratos, Keto, Mailpit; schemas; `make up` | — | NFR-05, NFR-06 |
| U2 | OpenAPI v1 skeleton (`/v1/me`, `/admin/v1/me`, problem model) | — | ADR-0010 |
| U3 | identity-service skeleton: config, slog, otel, health, migrations, `Authenticate` middleware, `GET /v1/me` | U1, U2 | FR-08, FR-09 |
| U4 | Mobile: sign in → call `/v1/me` → show email | U3, S1 | FR-02 |
| U5 | Admin web: login → `/admin/v1/me` → shell layout | U3, S2 | FR-05 |
| U6 | CI: lint + unit tests per app, gitleaks | U3–U5 | NFR-03, NFR-08 |

## Milestone 2 — Customer self-service

| # | Unit | Req |
| --- | --- | --- |
| U7 | Mobile registration + `continue_with` verification screen | FR-01, FR-03 |
| U8 | Webhook provisioning + lazy upsert | FR-09 |
| U9 | Recovery by code, change password (settings flow) | FR-04 |
| U10 | Profile edit (`PATCH /v1/me`), logout | FR-10, FR-13 |

## Milestone 3 — Admin plane

| # | Unit | Req |
| --- | --- | --- |
| U11 | Keto model + `Authorizer` adapter + bootstrap CLI | FR-06, FR-07 |
| U12 | TOTP enrolment + AAL2 step-up in web; AAL2 + age enforcement in API | FR-06 |
| U13 | Customers list/detail/disable/enable/revoke | FR-11 |
| U14 | Audit log write + view | FR-12 |
| U15 | Invite admins (own mailer, TOTP enrolment inside invite, 24 h deadline) + roles (enum mapping, last-super-admin guard) | FR-07 |

## Milestone 4 — Hardening & release readiness

| # | Unit | Req |
| --- | --- | --- |
| U16 | Integration tests (testcontainers), Playwright & mobile E2E | NFR-08 |
| U17 | Security checklist pass (06-security §6.3–6.6), rate limits | NFR-02, NFR-04 |
| U18 | k6 load test, session cache tuning | NFR-01 |
| U19 | Observability dashboards & alerts | NFR-07 |

## Later (not scheduled)

Social login on mobile (FR-14), passkeys for admins (FR-15), Ory Hydra for
third-party clients, Oathkeeper when a second service appears, i18n email templates.
