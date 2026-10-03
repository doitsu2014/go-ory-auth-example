# Intent Capture Questions

> Answer inline with `[Answer]: <letter> <optional text>`.
> Every question has lettered options plus `X. Other` if none fit.

## Q1: What is the primary outcome this project must deliver first?

A. A reusable identity/authentication platform (Ory Kratos + Keto + Hydra) that the admin web app and mobile app both consume, with the Go backend focused on domain/business logic.
B. A single Go backend service that owns auth itself and embeds Ory components as libraries, exposing one API to the clients.
C. A working end-to-end demo (login/register flows through Ory) that proves the stack, production-hardening comes later.
D. A production-grade system from day one: multi-environment, SSO, MFA, audit logging, and CI/CD included.
E. Something else — describe the top outcome you need.

[Answer]:

## Q2: Who are the users and what identity/access model do you need?

A. Two audiences: internal staff (admin web) and external customers (mobile). Role-based access (admin/operator/viewer) for staff, self-service registration for customers.
B. All users are internal staff; mobile app is for staff on the move. Single role model across both.
C. External customers on both web and mobile; admin console is for a small operations team.
D. Multiple tenant organisations, each with its own users and admins (multi-tenancy).
E. Other — describe the user types and roles.

[Answer]:

## Q3: Which Ory capabilities are actually in scope?

A. Kratos only (identity, registration, login, sessions, MFA) — start minimal.
B. Kratos + Hydra (OAuth2/OIDC provider) for standards-based tokens and third-party/API access.
C. Kratos + Keto (relationship-based authorization / permissions) for fine-grained access control.
D. Full Ory Stack: Kratos + Hydra + Keto (+ optionally Oathkeeper as the API gateway/zero-trust proxy).
E. Other / not sure — recommend a minimal viable combination.

[Answer]:

## Q4: What is explicitly OUT of scope for this phase?

A. Nothing beyond the docs/architecture deliverable described in the request — this phase only produces design docs.
B. Production deployment and infrastructure (this is design + local dev only).
C. Social/enterprise SSO (Google, GitHub, SAML/OIDC federation).
D. Payments, billing, or business-domain features beyond auth — identity only.
E. Other — list what must not be built now.

[Answer]:

## Q5: What are the hard constraints (timeline, team, budget, compliance)?

A. Solo developer, no fixed deadline, self-hosted Ory on my own Postgres — learning/architecture-first.
B. Small team (2–5), target MVP in weeks, must run locally with Docker Compose.
C. Compliance-sensitive (GDPR/PII, audit trails, data residency) — security and privacy rules are hard constraints.
D. Cloud-managed Ory (Ory Network) acceptable; I don't want to operate the control plane myself.
E. Other — describe deadlines, team size, or regulatory constraints.

[Answer]:
