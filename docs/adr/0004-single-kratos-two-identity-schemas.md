# 0004. One Kratos with `customer` and `admin` identity schemas

- Status: Proposed
- Date: 2026-10-03

## Context

Customers self-register on mobile; admins must never self-register and need
MFA. Kratos supports multiple identity schemas and only allows choosing those
marked `selfservice_selectable: true` in self-service flows
([docs](https://www.ory.com/docs/identities/model/identity-schema-selection)).

## Decision

- One Kratos deployment with two schemas: `customer` (default, selectable) and
  `admin` (non-selectable; created only via the admin API by invitation/bootstrap).
- identity-service maps `schema_id` → `Kind` and enforces planes:
  `/v1` customer only, `/admin/v1` admin only (+ AAL2 + Keto).
- **Required** hardening: an interrupting after-login `web_hook`
  (`can_interrupt: true`) that rejects `admin` identities on API flows and
  `customer` identities on browser flows, so neither population can get a
  session on the wrong client. Spike S3 confirms support in the pinned Kratos
  release. If it isn't supported, the API-side plane checks below remain the
  control and the gap is recorded as an accepted risk.
- identity-service binds each credential type to its plane: cookie only on
  `/admin/v1`, bearer only on `/v1`.
- The `admin` schema has no `code` login identifier, and the `code` method
  keeps `passwordless_enabled: false` and `mfa_enabled: false`.

## Alternatives considered

- **Two Kratos instances** (separate DBs, domains, cookies) — strongest
  isolation, independent session lifespans/policies; double the ops and config.
- **One schema + role flag in `metadata_public`** — simplest, but any customer
  becomes an admin by a single metadata write; mixes populations.

## Consequences

- Kratos session policy (lifespan, required AAL) is global → admin-specific
  rules (AAL2, 12 h max age) are enforced in identity-service.
- An email can't be both a customer and an admin (identifier uniqueness) —
  staff use a work email for admin.

## Revisit when

Regulatory/tenant isolation requires separate stores, or admin and customer
session policies diverge beyond what the service can enforce → move to two
Kratos instances (clients unaffected except admin base URL).
