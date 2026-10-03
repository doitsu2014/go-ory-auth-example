# 0006. Session validation in Go middleware with short cache (no Oathkeeper in v1)

- Status: Proposed
- Date: 2026-10-03

## Context

Every API request must be tied to a valid Kratos session. Options: call
Kratos from the service, or put Ory Oathkeeper in front to authenticate and
forward a signed JWT.

## Decision

identity-service authenticates in middleware by calling Kratos
`/sessions/whoami` (cookie or `X-Session-Token`) and caches positive results
in-process for `min(30 s, session expiry)`, keyed by SHA-256 of the credential.
Admin disable/revoke invalidates the local cache.

Kratos runs with `session.whoami.required_aal: highest_available`, so whoami
returns `403 session_aal2_required` for an AAL1 session of an identity that has
a second factor. The verifier maps this to `aal2_required` (step-up), never to
`401`. Only `401` from Kratos means "no session".

## Alternatives considered

- **Oathkeeper** (`cookie_session` / `bearer_token` authenticators → `id_token`
  mutator) — centralises authn for many services and enables offline JWT
  validation; one more hop and config surface for a single service.
- **No cache** — simplest, but every request costs a Kratos round trip (NFR-01 risk).
- **Shared Redis cache** — consistent invalidation across replicas; extra infra.

## Consequences

- Up to 30 s revocation lag on other replicas (accepted risk, documented).
- Middleware lives behind a `SessionVerifier` port → swapping to
  Oathkeeper-issued JWTs later is an adapter change.

## Revisit when

A second backend service needs the same authentication, or revocation must be
immediate across replicas.
