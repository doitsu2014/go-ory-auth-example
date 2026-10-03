# 0008. Profile provisioning via webhook + idempotent lazy upsert

- Status: Proposed
- Date: 2026-10-03

## Context

identity-service needs a `profile` row for each identity. Kratos can call a
`web_hook` after registration; webhooks can fail or be delayed.

## Decision

1. Kratos after-registration `web_hook` (non-interrupting, response ignored)
   → `POST /internal/hooks/kratos/after-registration` → `INSERT … ON CONFLICT DO NOTHING`.
2. Authenticated requests call `EnsureProfile` (same upsert) when the profile is missing.

## Alternatives considered

- **Webhook only (interrupting)** — registration fails if our service is
  down; couples Kratos availability to ours.
- **Lazy only** — simplest, but no hook point for future onboarding side effects (welcome email, analytics).
- **Poll Kratos admin API** — wasteful, delayed.

## Consequences

- Exactly one profile per identity regardless of retries/order.
- Webhook must authenticate (API key) and stay on the private port.

## Revisit when

Provisioning needs multi-step side effects → introduce an outbox/queue.
