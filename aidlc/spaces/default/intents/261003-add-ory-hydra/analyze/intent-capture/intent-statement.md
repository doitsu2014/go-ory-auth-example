# Intent Statement — Machine-to-machine access with Ory Hydra

## Problem

Back-office jobs and partner systems (billing, CRM sync, reporting) need to
call identity-service without a human session. Today the only credentials are
Kratos sessions (customer Bearer, admin cookie), which are tied to people. The
owner wants **Ory Hydra** as the OAuth2 server for **machine-to-machine**
access, using the standard `client_credentials` grant (RFC 6749 §4.4).

## Users

| User | Needs |
| --- | --- |
| Service client (job / partner system) | Get a short-lived access token with only the scopes it needs, then call a machine API |
| super_admin | Register, list, rotate secret, delete service clients from the admin web; the secret is shown once |
| Operator | Run Hydra in compose; rotate signing keys; see which client called what |

## Success criteria

1. `make up` also starts Hydra (Postgres-backed) and its migrations.
2. A client gets a JWT from `POST /oauth2/token` (client_credentials) and calls
   `/m2m/v1/*`; identity-service verifies it offline (JWKS), checks issuer,
   audience `identity-service`, expiry, and per-route scope.
3. Deleting a client stops its access within ≤ 30 s even though JWTs are still
   cryptographically valid.
4. Only super_admins manage clients; every change is audited; secrets never
   appear in logs or API responses after creation.
5. Machine credentials work only on `/m2m/v1/*`; session credentials never work there, and Hydra tokens never work on `/v1` or `/admin/v1`.

## In scope

Hydra in compose, Hydra DB provisioning, JWT verification in Go, a machine
plane with `customers:read` / `audit:read` scopes, service-client management
API + admin web page + CLI, smoke checks, docs and ADR.

## Out of scope

Authorization-code / OIDC login for third parties (no login/consent UI),
`private_key_jwt` client auth (documented as the production upgrade), machine
access to PII, mobile changes.
