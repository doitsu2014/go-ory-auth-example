# Intent Statement — go-ory-auth-example

## Problem

The owner wants one identity foundation shared by two first-party clients — a
React admin website and a Flutter mobile app — instead of hand-rolled login in
each. Identity must be handled by the Ory stack, persisted in PostgreSQL, and
fronted by a backend written in Go. Before writing code, the owner wants the
architecture and design principles written down in a `docs/` folder.

## Users

| User | Client | Needs |
| --- | --- | --- |
| Administrator / operator | React admin web | Sign in (no self sign-up), MFA, manage end users |
| End user (customer) | Flutter mobile app | Register, verify email, sign in, recover password, manage profile |
| Developer | Repo, local stack | Understand the design, run everything locally, extend safely |

## Success criteria (observable)

1. `docs/` contains the architecture (context, components, auth flows, data,
   deployment, security) and the design principles, reviewed by the owner.
2. (Later iterations) An admin signs in to the admin web; a customer registers
   and signs in from the mobile app; both call the Go identity service, which
   rejects requests without a valid Ory session.
3. All persistent state (Ory + app) lives in PostgreSQL.
4. The full stack starts locally with one command (`docker compose up`).

## In scope

- Ory Kratos (identity, self-service flows, sessions), Ory Keto (permissions).
- Go identity service (REST API, Kratos/Keto integration, user profile domain).
- React admin web (login only), Flutter mobile app (login + registration).
- PostgreSQL for Ory and application data; local Docker Compose topology.

## Out of scope (first iteration)

- Third-party OAuth2/OIDC clients (Ory Hydra) — deferred, with a documented trigger.
- Multi-tenancy, billing, production cloud infrastructure, social login.

## Constraints

- Backend language: Go. Database: PostgreSQL. Identity: Ory stack.
- Admin web: React. Mobile: Flutter.
- Secrets never in source or logs (org rule); least privilege for credentials.

## Assumptions

- First-party clients only, so Ory session-based auth is sufficient (no OAuth2 needed).
- Self-hosted open-source Ory; Ory Network is an equivalent drop-in alternative.
- One team, monorepo.

## Cost of doing nothing

Each client would implement its own credential handling, duplicating
security-critical code and diverging in behaviour.
