# 0007. One PostgreSQL cluster, database + role per component

- Status: Proposed
- Date: 2026-10-03

## Context

Kratos, Keto and identity-service all need PostgreSQL. Ory manages its own
schema via migrations that change between releases.

## Decision

One PostgreSQL 16+ cluster with databases `kratos`, `keto`, `identity`, each
with its own login role(s). identity-service uses a DDL role
(`identity_migrator`) for migrations and a DML-only role (`identity_app`) at
runtime. No component connects to another's database.

## Alternatives considered

- **Single database, shared schema** — risk of name clashes and accidental
  coupling to Ory tables.
- **Separate clusters per component** — stronger blast-radius isolation,
  more cost/ops; justified at larger scale.

## Consequences

- Cheap to run locally and in small production setups.
- Shared cluster resources (connections, IO) → set per-role connection limits.
- Splitting into clusters later is a DSN change.

## Revisit when

One component's load or compliance needs isolate it.
