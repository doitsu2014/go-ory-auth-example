# 0012. Ory Hydra for machine-to-machine access

- Status: Proposed
- Date: 2026-10-03
- Revisits: [ADR-0002](0002-kratos-sessions-over-oauth2.md) ("machine-to-machine clients appear")

## Context

Back-office jobs and partner systems need API access without a person.
Kratos sessions belong to identities (people), and sharing a human account
with a job breaks auditing and least privilege.

## Decision

- Deploy **Ory Hydra** only as an OAuth2 authorisation server for the
  **client_credentials** grant (RFC 6749 §4.4). It has no login/consent app,
  and first-party apps keep using Kratos sessions.
- Access tokens are **JWTs** (RS256, 5 min). identity-service validates them
  offline using the JWKS, and checks client status through a 30 s cached
  lookup on Hydra admin so that delete and rotate take effect.
- A separate **`/m2m/v1` plane** with per-route OAuth2 scopes, PII-free DTOs,
  and an allowlisted audit feed.
- Clients are managed only through identity-service (super_admin, audited,
  secret shown once). Hydra is the single store, and its metadata marks the
  clients we own.

## Alternatives considered

- **Opaque tokens + introspection per request**: immediate revocation, but a
  Hydra round trip on every call and Hydra becomes a hard runtime dependency.
- **Static API keys in our DB**: simpler, but non-standard. It has no expiry,
  no scopes on the wire, and no path to partner OAuth2.
- **Kratos "service" identities**: these mix machines into the people store,
  and Kratos has no client_credentials grant.

## Consequences

- One more Ory component, with its own DB and migrations.
- Revocation lag is ≤ 30 s per replica, the same bound as the session cache.
- Third-party *user* delegation (authorization code + consent) becomes a later
  increment on the same Hydra.

## Revisit when

- Partners onboard: move to `private_key_jwt` and consider mTLS-bound tokens
  (RFC 8705).
- Immediate revocation is required: use introspection or a shared revocation
  cache.
