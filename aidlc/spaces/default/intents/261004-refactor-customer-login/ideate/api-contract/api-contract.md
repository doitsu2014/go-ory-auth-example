# API contract: customer login proxy

The source of truth is `api/openapi/identity-service.v1.yaml`, which has been updated.

| Route | Request | 200 | Errors |
| --- | --- | --- | --- |
| `POST /v1/auth/login` | `CustomerCredentials {login:{type,value}, password}` | `CustomerAuthSession {session_token, session}` | 400 `auth_flow_rejected`, 422 `validation_failed` (`login.type`, `login.value`, `password`), 429, 503 |
| `POST /v1/auth/registration` | `CustomerCredentials` | `CustomerAuthSession {…, verification_flow_id?}` | same |
| `POST /v1/auth/recovery` | `CustomerRecoveryRequest {login}` | `CustomerRecoveryStarted {flow_id}` | 422, 429, 503 (400 only if Kratos rejects the flow) |
| ~~`POST /v1/auth/identifiers`~~ | removed | | 404 |

## `auth_flow_rejected`

```json
{ "type": ".../problems/auth-flow-rejected", "title": "Authentication rejected", "status": 400,
  "code": "auth_flow_rejected",
  "errors": [{ "field": "form", "code": "4000006" }] }
```

`field` mapping from the Kratos node name:

- `identifier`, `traits.login_id`, `email` → `login`
- `password` → `password`
- anything else, including `ui.messages` → `form`

Only messages of type `error` are included. Kratos text, context and node
values are never forwarded.

## Unchanged

The webhook contracts (pre-registration, after-registration, after-login and
courier) and `/v1/me` are unchanged. The pre-registration description now
refers to the stored handle.
