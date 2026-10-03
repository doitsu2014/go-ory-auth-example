# 0002. Kratos sessions instead of OAuth2/OIDC (Hydra) for first-party apps

- Status: Proposed
- Date: 2026-10-03

## Context

Both clients (admin web, mobile app) are first-party. The Ory stack offers
Kratos (identity + sessions) and Hydra (OAuth2/OIDC server). Ory's own guidance
is that first-party apps usually don't need OAuth2; OAuth2 on mobile requires
the system browser and adds token/consent lifecycle complexity.
([Ory blog](https://www.ory.com/blog/oauth2-openid-connect-do-you-need-use-cases-examples))

## Decision

Use **Kratos sessions** as the only credential in v1: cookie for the admin web,
session token for the mobile app. identity-service validates sessions via
Kratos `/sessions/whoami`. Hydra is **not** deployed.

## Alternatives considered

- **Kratos + Hydra for both clients** (Authorization Code + PKCE, JWT access
  tokens): standard tokens, offline JWT validation, ready for third parties —
  but browser redirect on mobile, consent app to build, refresh-token handling
  in both clients, more moving parts.
- **Hydra only with custom login** — rebuilds what Kratos already does.

## Consequences

- Simpler clients and native mobile UX (no browser hop).
- API validation needs a Kratos round trip (mitigated by caching, ADR-0006).
- Tokens are opaque; other services must also ask Kratos (or a gateway).

## Revisit when

A third-party client, "Sign in with <our product>", SSO across independent
products, or machine-to-machine clients appear. Path: add Hydra with Kratos as
login/consent provider; identity-service additionally accepts Hydra JWTs; existing
clients unchanged.
