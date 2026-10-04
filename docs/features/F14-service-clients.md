# F14 — Service clients (OAuth2 client credentials)

A super_admin registers partner services as Hydra OAuth2 clients
(`client_credentials`, JWT access tokens). identity-service creates only
**managed** clients. It tags each one with an HMAC integrity tag, so clients
created outside the service stay invisible. Hydra shows the secret **once**,
on create or rotate.

## Actors and entry points

| Endpoint | Keto | Notes |
| --- | --- | --- |
| `GET /admin/v1/service-clients` | `manage_service_clients` | managed clients only, never secrets |
| `POST /admin/v1/service-clients` `{name, owner, scopes}` | `manage_service_clients` | `Idempotency-Key` required, 201 with `client_secret` |
| `GET /admin/v1/service-clients/{client_id}` | `manage_service_clients` | not used by admin web |
| `POST /admin/v1/service-clients/{client_id}/rotate-secret` | `manage_service_clients` | 200 with new secret |
| `DELETE /admin/v1/service-clients/{client_id}` | `manage_service_clients` | 204 |
| CLI `identity-service clients create` | system | no Keto, no idempotency |

Scopes form a closed set: `customers:read`, `audit:read`.

## Functional diagram

```mermaid
flowchart TD
  A[Admin action] --> G["Admin Guard + Keto manage_service_clients (super_admin)"]
  G --> T{Action}
  T -- create --> C1{"name 3-64 a-z0-9-, owner email, scopes known"}
  C1 -- invalid --> E422[422]
  C1 -- ok --> C2{"Reserve Idempotency-Key"}
  C2 -- "replay completed" --> C2r["201 stored body WITHOUT secret"]
  C2 -- "mismatch / pending" --> E409[409]
  C2 -- reserved --> C3["Hydra POST /admin/clients (jwt, client_secret_basic, metadata + integrity)"]
  C3 -- "409 / other 4xx" --> C3r["release key → 409 / error (nothing to delete)"]
  C3 -- "ambiguous (5xx, transport) or response not managed" --> C3x["Hydra DELETE (compensate) + release → error"]
  C3 -- ok --> C4["TX: audit service_client.created + complete key"]
  C4 -- fail --> C3x
  C4 -- ok --> C5["201 with client_secret → SecretDialog (must tick stored)"]
  T -- rotate --> R1["audit secret_rotation_started"]
  R1 --> R2["Hydra PATCH client_secret + metadata.tokens_valid_after = ceil(now)+1s"]
  R2 --> R3[Verifier.Invalidate client]
  R3 --> R4{"PATCH ok?"}
  R4 -- yes --> R5["audit secret_rotated → 200 new secret"]
  R4 -- no --> R6["audit secret_rotation_failed → error"]
  T -- delete --> D1["audit deletion_started → Hydra DELETE → invalidate → deleted / deletion_failed"]
  T -- list --> L1["Hydra GET /admin/clients (≤ 20 pages) → keep managed → sort"]
```

## Sequence — create

```mermaid
sequenceDiagram
  autonumber
  participant AW as Admin web
  participant IS as identity-service
  participant DB as identity DB
  participant HY as Hydra admin :4445

  AW->>IS: POST /admin/v1/service-clients {name, owner, scopes} + Idempotency-Key
  IS->>IS: Guard (Keto manage_service_clients), strict body, NewRegistration (422)
  IS->>DB: INSERT idempotency_key (sha256 op+body) ON CONFLICT DO NOTHING
  IS->>HY: POST /admin/clients {client_id: uuid, grant_types:[client_credentials], scope, audience:[identity-service], token_endpoint_auth_method: client_secret_basic, access_token_strategy: jwt, metadata{managed_by, owner, created_by, integrity}}
  HY-->>IS: 201 client + generated client_secret
  rect rgba(128,128,128,0.12)
    IS->>DB: INSERT audit_event service_client.created {name, scopes}
    IS->>DB: UPDATE idempotency_key 201 body (without secret)
    IS->>DB: COMMIT
  end
  IS-->>AW: 201 {client_id, name, owner, scopes, client_secret}
  AW->>AW: SecretDialog shows secret once + TokenExample curl
```

## Sequence — rotate secret

```mermaid
sequenceDiagram
  autonumber
  participant AW as Admin web
  participant IS as identity-service
  participant DB as identity DB
  participant HY as Hydra admin
  participant VC as Verifier status cache

  AW->>IS: POST /admin/v1/service-clients/{client_id}/rotate-secret
  IS->>HY: GET /admin/clients/{id} (must be managed, else 404)
  IS->>IS: secret = 32 random bytes base64url
  IS->>DB: INSERT audit_event service_client.secret_rotation_started (commit)
  IS->>HY: PATCH /admin/clients/{id} [replace /client_secret, add /metadata/tokens_valid_after]
  IS->>VC: Invalidate(client_id)
  alt PATCH ok
    IS->>DB: INSERT audit_event service_client.secret_rotated
    IS-->>AW: 200 {client_secret}
  else PATCH failed
    IS->>DB: INSERT audit_event service_client.secret_rotation_failed
    IS-->>AW: 404 / 503
  end
  Note over VC: tokens with iat < tokens_valid_after are now rejected (F15)
```

## Code references

- `services/identity-service/internal/app/serviceclients.go:66` — `List`, `:121` `Create`, `:212` `RotateSecret`, `:242` delete, `:263` `twoPhase`
- `services/identity-service/internal/adapter/hydra/admin.go:111` — `managed`, `:149` create, `:236` list
- `services/identity-service/internal/domain/machine/machine.go:22` — scopes, `:96` validation
- `apps/admin-web/src/features/service-clients/CreateServiceClientDialog.tsx:55`, `SecretDialog.tsx`
- `deploy/ory/hydra/hydra.yml`

## Related

[ADR-0012](../adr/0012-hydra-for-machine-to-machine.md) ·
[09 Machine access](../architecture/09-machine-access.md)
