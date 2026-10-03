# 0001. Monorepo for services, apps, infra and docs

- Status: Proposed
- Date: 2026-10-03

## Context

One team builds a Go service, a React app, a Flutter app and the Ory
configuration they all depend on. The OpenAPI contract is shared by all three.

## Decision

Single repository with top-level `services/`, `apps/`, `api/`, `deploy/`, `docs/`
(see [repository structure](../repository-structure.md)). CI uses path
filters per app. Trunk-based development with short-lived branches.

## Alternatives considered

- **Polyrepo (one per app)** — independent release cadence, but contract
  changes need coordinated PRs across repos and the local stack is harder to
  assemble.

## Consequences

- One PR can change the contract, server and clients atomically.
- CI must stay fast via path filters and caching.
- Tooling per ecosystem coexists (Go modules, pnpm, pub) — no forced single build tool; a root `Makefile` orchestrates.

## Revisit when

Separate teams own the mobile app and the backend with different release processes.
