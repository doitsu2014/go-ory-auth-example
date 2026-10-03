# API Contract

Normative human-readable contract: `docs/api/identity-service-api.md`.
Conventions (errors, pagination, idempotency, versioning): `docs/principles/05-api-guidelines.md`.
Machine contract to be authored first in Develop: `api/openapi/identity-service.v1.yaml` (unit U2).

## Interfaces enumerated

1. Customer REST `/v1/*` — `GET /v1/me`, `PATCH /v1/me`.
2. Admin REST `/admin/v1/*` — me, customers (list/get/disable/enable/revoke sessions), admins (list/invite/role), audit events.
3. Kratos webhook `/internal/hooks/kratos/after-registration` (private port, API key).
4. Health/metrics (private).
5. CLI `identity-service admin bootstrap`.
6. Data schemas: Kratos identity schemas `customer.v1`, `admin.v1`; Keto OPL `Console` namespace; identity DB DDL.
7. Consumed: Kratos public & admin APIs, Keto read/write APIs (Ory contracts).

## Per-contract coverage

Each endpoint in the API doc states method, path, auth, permission, request,
response, errors and at least one request/response example; error model is
RFC 9457 with stable `code`; list endpoints use cursor pagination; creates use
`Idempotency-Key`; actions are idempotent by state.

## Compatibility

URL major versioning; additive-only within v1; `oasdiff breaking` in CI;
`Deprecation`/`Sunset` headers with ≥ 6 months for mobile.
