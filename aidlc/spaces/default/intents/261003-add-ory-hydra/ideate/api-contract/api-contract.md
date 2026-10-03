# API Contract — M2M

Source of truth: `api/openapi/identity-service.v1.yaml`.

| Method | Path | Auth | Policy | Success | Errors |
| --- | --- | --- | --- | --- | --- |
| GET | `/m2m/v1/customers/{id}` | Hydra JWT | scope `customers:read` | 200 `MachineCustomer` | 401 `invalid_token`, 403 `insufficient_scope`, 404, 429, 503 |
| GET | `/m2m/v1/audit-events` | Hydra JWT | scope `audit:read` | 200 `AuditEventPage` | 401, 403, 429, 503 |
| GET | `/admin/v1/service-clients` | cookie AAL2 | `manage_service_clients` | 200 `ServiceClientPage` | 401, 403 |
| POST | `/admin/v1/service-clients` | cookie AAL2 + `Idempotency-Key` | `manage_service_clients` | 201 `ServiceClientWithSecret` | 401, 403, 409, 422, 503 |
| GET | `/admin/v1/service-clients/{client_id}` | cookie AAL2 | `manage_service_clients` | 200 `ServiceClient` | 404 |
| POST | `/admin/v1/service-clients/{client_id}/rotate-secret` | cookie AAL2 | `manage_service_clients` | 200 `ServiceClientWithSecret` | 404, 503 |
| DELETE | `/admin/v1/service-clients/{client_id}` | cookie AAL2 | `manage_service_clients` | 204 | 404, 503 |

- `MachineCustomer`: `id`, `state`, `email_verified`, `created_at`. No email, name or PII.
- `ServiceClient`: `client_id`, `name`, `owner`, `scopes[]`, `created_at`,
  `created_by`. `ServiceClientWithSecret` = `ServiceClient` + `client_secret`
  (shown once).
- `scopes` is a closed enum `[customers:read, audit:read]`, with min 1 item.
  `name` is 3–64 chars `^[a-z0-9][a-z0-9-]*$`. `owner` is an email.
- Problem codes: `invalid_token`, `insufficient_scope` (new). Machine-plane
  401/403 also send `WWW-Authenticate: Bearer error="…"` (RFC 6750 §3).
- Audit actions: `service_client.created`, `service_client.secret_rotated`,
  `service_client.deleted`. Details hold name and scopes, never the secret.
- `Permission` enum gains `manage_service_clients`.
- Token endpoint (Hydra, not our contract): `POST http://localhost:4444/oauth2/token`
  with `grant_type=client_credentials&scope=…&audience=identity-service` and
  HTTP Basic client credentials.
