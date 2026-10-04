# API Guidelines (identity-service)

## 1. Style

- REST over HTTPS, JSON (`application/json; charset=utf-8`), OpenAPI **3.0.3** (best generator support; move to 3.1 once oapi-codegen fully supports it)
  as the source of truth (`api/openapi/identity-service.v1.yaml`).
- Resource-oriented plural nouns: `/admin/v1/customers/{id}`.
- Actions that aren't CRUD use a sub-resource verb: `POST /admin/v1/customers/{id}/disable`.
- `snake_case` JSON fields; RFC 3339 UTC timestamps (`2026-10-03T08:00:00Z`); UUIDs as strings.
- Two planes: `/v1/*` (customer, caller-scoped — no foreign ids in paths) and `/admin/v1/*`.

## 2. Authentication

| Client | Header |
| --- | --- |
| Mobile | `Authorization: Bearer <kratos_session_token>` |
| Admin web | Cookie `ory_kratos_session` (sent by the browser) |

Both are resolved through Kratos; no other credential types in v1.

## 3. Errors — RFC 9457 Problem Details

```json
HTTP/1.1 403 Forbidden
Content-Type: application/problem+json

{
  "type": "https://docs.example.com/problems/aal2-required",
  "title": "Second factor required",
  "status": 403,
  "code": "aal2_required",
  "detail": "This action requires multi-factor authentication.",
  "request_id": "01J9Z6Q3V5H8C2N1X0T4R7M6KD"
}
```

Validation errors add `errors: [{ "field": "display_name", "code": "too_long" }]`.

Standard codes: `unauthenticated` (401), `forbidden`, `not_admin`,
`aal2_required`, `mfa_enrollment_required`, `email_not_verified` (403),
`not_found` (404), `conflict` (409), `validation_failed` (422),
`rate_limited` (429), `dependency_unavailable` (503), `internal` (500).
Customer auth (`/v1/auth/*`, ADR-0014) adds:

- `auth_flow_rejected` (400): Kratos rejected the flow; `errors[].code` is the
  Kratos message id, and `field` is `login`, `password` or `form`;
- `auth_flow_expired` (410): the recovery flow expired.

## 4. Pagination, filtering, sorting

- Cursor pagination: `?page_size=50&page_token=<opaque>`; response
  `{ "items": [...], "next_page_token": "..." }` (absent on last page).
- `page_size` default 25, max 100.
- Filters as explicit query params (`?email=`, `?state=active`); no generic query language.

## 5. Idempotency & concurrency

- `GET`, `PUT`, `DELETE`, `PATCH /v1/me` are idempotent.
- Creating `POST`s (`/admin/v1/admins`) require `Idempotency-Key` (UUID); repeats within 24 h return the stored response.
- Action `POST`s (`/disable`, `/enable`) are idempotent by state (disabling a disabled customer → `200`).
- Optimistic concurrency for profile updates via `ETag` / `If-Match` (optional in v1).

## 6. Versioning & compatibility

- Major version in the path (`/v1`). Within a major version only **additive** changes:
  new endpoints, new optional fields, new enum values (clients must tolerate unknown values).
- Breaking = removing/renaming fields, changing types/semantics, making optional
  required, changing auth. Breaking changes ship as `/v2` alongside `/v1`.
- Deprecation: `Deprecation` + `Sunset` headers, ≥ 6 months for mobile (old app versions in the wild).
- CI runs `oasdiff breaking` against `main`.

## 7. Security headers & limits

- `Cache-Control: no-store` on authenticated responses.
- Rate limits at the ingress; `429` with `Retry-After`.
- Request body ≤ 1 MiB.
- Request id: accept `X-Request-Id`, always return it.
