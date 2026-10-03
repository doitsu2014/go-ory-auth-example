# Requirements

Source key: **IS** = intent statement; **Q*n*** = requirements-questions answer *n*; **ORG** = org memory.

## Functional

| ID | Statement | Pri | Source | Acceptance criteria | Verify |
| --- | --- | --- | --- | --- | --- |
| FR-01 | A customer can register from the mobile app with email + password | M | IS | Given a new email, the registration native flow returns a session token and an identity with schema `customer` exists | E2E (mobile → Kratos) |
| FR-02 | A customer can sign in from the mobile app | M | IS | Valid credentials return a session token; invalid return a flow error, no token | E2E |
| FR-03 | A customer verifies their email by one-time code | M | Q1 | After registration a code is sent; submitting it marks the address `verified` | Integration (Mailpit) |
| FR-04 | A customer can recover a forgotten password by code | M | Q1 | Recovery flow issues a code and lets the user set a new password | Integration |
| FR-05 | An admin can sign in to the admin web (no registration UI) | M | IS, Q2 | Admin identity signs in via browser flow; registration route is absent in admin web | E2E (Playwright) |
| FR-06 | Admin endpoints require an admin role and AAL2 (MFA) | M | Q3, ORG | A customer session or an admin session at AAL1 gets 403 on `/admin/*` | Integration |
| FR-07 | Admins are created only by invitation | M | Q2 | Bootstrap CLI or `POST /admin/v1/admins` creates an `admin` identity and returns/sends a recovery link | Integration |
| FR-08 | The Go API authenticates every request through an Ory session | M | IS | No/invalid cookie or token → 401; valid → request carries identity id | Unit + integration |
| FR-09 | The identity service keeps a user profile keyed by Kratos identity id | M | Q5 | First authenticated request or registration webhook creates exactly one profile row (idempotent) | Integration (Postgres) |
| FR-10 | Customer can view/update own profile from the mobile app | M | Q5 | `GET/PATCH /v1/me` returns/updates only the caller's profile | Integration |
| FR-11 | Admin can list, search, view, disable and re-enable customers | S | IS | Disabled customer's sessions are revoked and new logins fail | Integration |
| FR-12 | Every admin mutation is written to an audit log | S | Q5, ORG | Audit row with actor, action, target, timestamp, request id | Integration |
| FR-13 | Users can sign out (revoke session) on both clients | M | IS | After logout the token/cookie is rejected by the API | E2E |
| FR-14 | Social sign-in (Google, Apple) on mobile | C | Q1 | — deferred | — |
| FR-15 | Passkeys (WebAuthn) for admins | C | Q1 | — deferred | — |

## Non-functional

| ID | Statement | Pri | Source | Acceptance criteria | Verify |
| --- | --- | --- | --- | --- | --- |
| NFR-01 | Authenticated API p95 latency < 150 ms at 50 RPS on a dev laptop (excluding login hashing) | S | Q4 | k6 run report | Load test |
| NFR-02 | Security bar: OWASP ASVS v4 Level 2 for auth & session controls | M | Q4 | Checklist in security doc is satisfied | Review |
| NFR-03 | Secrets never in source or logs | M | ORG | gitleaks clean; log redaction tests | CI |
| NFR-04 | Least privilege: Kratos/Keto admin APIs are never reachable from the internet | M | ORG | Admin ports only on the private network in compose / k8s | Review + test |
| NFR-05 | Whole stack starts locally with one command in < 2 min | M | IS, Q4 | `docker compose up` → health checks green | Manual/CI |
| NFR-06 | All persistent state in PostgreSQL; each component owns its own database | M | IS | No component reads another component's tables | Review |
| NFR-07 | Structured logs, traces and metrics (OpenTelemetry) with request id correlation | S | — (derived from IS "build properly") | Trace spans across API → Kratos | Manual |
| NFR-08 | Every requirement has a test; happy-path floor per component | M | ORG | CI test report | CI |

## Constraints

| ID | Statement | Source |
| --- | --- | --- |
| C-01 | Backend in Go | IS |
| C-02 | PostgreSQL | IS |
| C-03 | Ory stack for identity | IS |
| C-04 | Admin web in React; mobile in Flutter | IS |
| C-05 | Human review before merge | ORG |

## Assumptions

| ID | Statement |
| --- | --- |
| A-01 | Only first-party clients → session auth, no OAuth2 server needed initially |
| A-02 | Single region, single team, monorepo |
| A-03 | Email is the only identifier in v1 (no phone/SMS) |

## Traceability check

All FR/NFR trace to IS, Q*n* or ORG. No orphans. FR-14/15 are Could and carry no acceptance criteria yet — they are not admitted to the build backlog until they do.
