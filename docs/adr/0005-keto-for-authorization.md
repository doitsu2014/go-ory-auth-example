# 0005. Ory Keto for authorization

- Status: Proposed
- Date: 2026-10-03

## Context

Admins have roles (`super_admin`, `admin`, `support`) with different
permissions. Later we may need relationship-based rules (e.g. a support agent
assigned to a region or a customer group).

## Decision

Use **Ory Keto** with an OPL model (`Console` namespace, see
[components](../architecture/02-components.md#22-ory-keto--authorization)).
identity-service calls `check` per protected use case and writes tuples on
invite / role change. Clients never talk to Keto.

## Alternatives considered

- **Roles in Kratos `metadata_public` + code-level RBAC** — no extra service;
  fine for 3 static roles, but no relationships, and role changes require
  identity writes.
- **Roles table in the identity DB** — simple, but re-implements what Keto
  does and drifts from "Ory stack" intent.
- **OPA / Casbin** — capable, but outside the Ory ecosystem.

## Consequences

- One more container + database; a network call per authorization (Keto check
  is fast; cache per request).
- Permission model is code-reviewed (`namespaces.keto.ts`).

## Revisit when

Keto latency/ops cost outweighs value and roles stay static → fall back to
metadata roles behind the same `Authorizer` port.
