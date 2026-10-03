# identity-service API — v1 contract

Human-readable contract. The normative version will be
`api/openapi/identity-service.v1.yaml` (written first in Develop, ADR-0010).
Conventions: [API guidelines](../principles/05-api-guidelines.md).

Base URLs: `https://api.example.com` (prod), `http://localhost:8080` (local).

## Interface inventory

| Interface | Kind | Consumer | Exposure |
| --- | --- | --- | --- |
| Customer API `/v1/*` | REST | Mobile app | Public |
| Admin API `/admin/v1/*` | REST | Admin web | Public (cookie, AAL2) |
| Kratos webhooks `/internal/hooks/kratos/*` | REST | Kratos | Private `:8081` |
| Health / metrics | REST | Orchestrator, Prometheus | Private |
| CLI `identity-service admin bootstrap` | CLI | Operator | Shell |
| Kratos public API | Ory | Both clients | Public (Ory contract, not ours) |

## Endpoint summary

| Method & path | Auth | Permission | Req |
| --- | --- | --- | --- |
| `GET /v1/me` | customer | self | FR-09, FR-10 |
| `PATCH /v1/me` | customer (verified email) | self | FR-10 |
| `GET /admin/v1/me` | admin (AAL1 allowed) | — | FR-05, FR-06 |
| `GET /admin/v1/customers` | admin AAL2 | `view_customers` | FR-11 |
| `GET /admin/v1/customers/{id}` | admin AAL2 | `view_customers` | FR-11 |
| `POST /admin/v1/customers/{id}/disable` | admin AAL2 | `manage_customers` | FR-11, FR-12 |
| `POST /admin/v1/customers/{id}/enable` | admin AAL2 | `manage_customers` | FR-11, FR-12 |
| `DELETE /admin/v1/customers/{id}/sessions` | admin AAL2 | `manage_customers` | FR-11, FR-12 |
| `GET /admin/v1/admins` | admin AAL2 | `manage_admins` | FR-07 |
| `POST /admin/v1/admins` | admin AAL2 + `Idempotency-Key` | `manage_admins` | FR-07, FR-12 |
| `PUT /admin/v1/admins/{id}/role` | admin AAL2 | `manage_admins` | FR-12 |
| `GET /admin/v1/audit-events` | admin AAL2 | `view_audit` | FR-12 |
| `GET /m2m/v1/customers/{id}` | Hydra JWT | scope `customers:read` | M2M-FR-05 |
| `GET /m2m/v1/audit-events` | Hydra JWT | scope `audit:read` | M2M-FR-06 |
| `GET/POST /admin/v1/service-clients` | admin AAL2 (+ `Idempotency-Key` on POST) | `manage_service_clients` | M2M-FR-08, 09 |
| `GET/DELETE /admin/v1/service-clients/{client_id}` | admin AAL2 | `manage_service_clients` | M2M-FR-09, 11 |
| `POST /admin/v1/service-clients/{client_id}/rotate-secret` | admin AAL2 | `manage_service_clients` | M2M-FR-10 |
| `GET /v1/me/personal-info` | customer | self | PII-FR-01 |
| `PUT /v1/me/personal-info` | customer (verified email) | self | PII-FR-02, 03 |
| `DELETE /v1/me/personal-info` | customer | self | PII-FR-04 |
| `POST /admin/v1/customers/lookup` | admin AAL2, 30/min | `view_customers` | PII-FR-07 |
| `GET /admin/v1/customers/{id}/personal-info` | admin AAL2 | `view_customers` (masked) | PII-FR-05 |
| `POST /admin/v1/customers/{id}/personal-info/reveal` | admin AAL2, 20/h | `reveal_customer_pii` | PII-FR-06 |
| `POST /internal/hooks/kratos/after-registration` | API key | — | FR-09 |
| `GET /healthz`, `GET /readyz`, `GET /metrics` | none (ops port `:9090`, private) | — | NFR-07 |

Credential per plane: `/m2m/v1/*` accepts only Hydra JWTs (see [09-machine-access](../architecture/09-machine-access.md)). `/v1/*` accepts only `Authorization: Bearer`.
`/admin/v1/*` accepts only the Kratos session cookie. Anything else gets `401`.
Every `/admin/v1/customers/{id}/*` endpoint returns `404` when the target isn't a
`customer` identity.

---

## `GET /v1/me`

```http
GET /v1/me
Authorization: Bearer ory_st_Ab12…
```

```json
200 OK
{
  "id": "5d9c2c61-6a1e-4b8f-9b8a-2f9d6f0c1e11",
  "email": "an.nguyen@example.com",
  "email_verified": true,
  "display_name": "An",
  "avatar_url": null,
  "locale": "vi-VN",
  "created_at": "2026-10-03T08:00:00Z"
}
```

`email`, `email_verified` come from the Kratos session; the rest from
`profile`. `name` is deprecated and never returned: the customer's real name is
encrypted personal info (`GET/PUT /v1/me/personal-info`, field `name`), and
`display_name` is only an optional nickname. Changing email/password goes
through the Kratos **settings flow**, not this API.

## `PATCH /v1/me`

```json
PATCH /v1/me
{ "display_name": "An N.", "locale": "en-US" }
```

`200` with the full `Me` object. Errors: `422 validation_failed`,
`403 email_not_verified`.

```json
422 Unprocessable Content
Content-Type: application/problem+json
{
  "type": "https://docs.example.com/problems/validation-failed",
  "title": "Validation failed", "status": 422, "code": "validation_failed",
  "errors": [{ "field": "display_name", "code": "too_long" }],
  "request_id": "01J9Z6…"
}
```

## `GET /admin/v1/me`

```json
200 OK
{
  "id": "7f3c…", "email": "ops@example.com", "aal": "aal2",
  "roles": ["admin"],
  "permissions": ["view_customers", "manage_customers", "view_audit"],
  "session_expires_at": "2026-10-03T20:00:00Z"
}
```

`403` codes: `not_admin`, `mfa_enrollment_required`, `aal2_required` (the web
uses these to route; see auth flows §3.4).

## `GET /admin/v1/customers`

Query: `email` (exact, via Kratos `credentials_identifier`), `state`
(`active|inactive`), `page_size`, `page_token`.

```json
200 OK
{
  "items": [
    { "id": "5d9c…", "email": "an.nguyen@example.com", "email_verified": true,
      "state": "active", "display_name": "An", "created_at": "2026-10-03T08:00:00Z" }
  ],
  "next_page_token": "eyJr…"
}
```

## `POST /admin/v1/customers/{id}/disable`

```json
POST /admin/v1/customers/5d9c…/disable
{ "reason": "fraud report #123" }
```

`200 { "id": "5d9c…", "state": "inactive" }` — idempotent. Side effects: Kratos
state → `inactive`, all sessions revoked, audit `customer.disabled`.
Errors: `404 not_found` (also for admin ids — admins aren't managed here).

## `POST /admin/v1/admins`

```http
POST /admin/v1/admins
Idempotency-Key: 3b0f5c1e-…
Content-Type: application/json

{ "email": "new.admin@example.com", "name": { "first": "Binh", "last": "Tran" }, "role": "support" }
```

```json
201 Created
{ "id": "a19e…", "email": "new.admin@example.com", "role": "support", "invitation_expires_at": "2026-10-04T08:00:00Z" }
```

identity-service creates the invitation code/link with Kratos admin
`POST /admin/recovery/code` (`expires_in` ≤ 24h). Kratos returns the link and
does **not** email it, so identity-service sends it through its own mailer. The
link is never returned in this response and never logged. `role` is a closed
enum (`support | admin | super_admin`) that the server maps to a Keto relation.
Errors: `409 conflict` (identifier exists), `422`, `403 forbidden`.

## `PUT /admin/v1/admins/{id}/role`

`{ "role": "admin" }` → `200`. Rules: the target must have `schema_id == admin`
(otherwise `404`). Callers can't change their own role (`403`). Removing the
last `super_admin` returns `409 conflict`. Audit `admin.role_changed`.

## `GET /admin/v1/audit-events`

Query: `target_type`, `target_id`, `actor_id`, `page_size`, `page_token`.

```json
200 OK
{ "items": [ { "id": 42, "occurred_at": "2026-10-03T09:12:00Z", "actor_id": "7f3c…",
  "action": "customer.disabled", "target_type": "customer", "target_id": "5d9c…",
  "request_id": "01J9Z6…", "details": { "reason": "fraud report #123" } } ] }
```

## `POST /internal/hooks/kratos/after-registration`

```http
POST /internal/hooks/kratos/after-registration
Authorization: <KRATOS_WEBHOOK_API_KEY>
{ "identity_id": "5d9c…", "schema_id": "customer", "flow_type": "api" }
```

`204 No Content` (idempotent). `401` on bad key. Served only on port `:8081`,
which only Kratos can reach.

Kratos config. Kratos does **not** expand `${VAR}` in YAML: the key value is
injected at deploy time (config rendered from the secret store, or the
path-derived env var), see [05-deployment §5.3](../architecture/05-deployment.md#53-configuration--secrets).

```yaml
selfservice:
  flows:
    registration:
      after:
        password:
          hooks:
            - hook: web_hook
              config:
                url: http://identity-service:8081/internal/hooks/kratos/after-registration
                method: POST
                body: file:///etc/config/kratos/webhooks/after-registration.jsonnet
                response: { ignore: true }
                auth:
                  type: api_key
                  config: { name: Authorization, value: "<rendered from secret store at deploy>", in: header }
            - hook: session
```

## Compatibility

Additive changes only within v1; breaking changes ship `/v2`; deprecations use
`Deprecation`/`Sunset` headers with ≥ 6 months notice.
