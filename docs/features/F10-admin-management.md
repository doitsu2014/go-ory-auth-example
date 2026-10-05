# F10 — Admin management (list, invite, change role)

A **super_admin** manages console users. An invite creates a Kratos admin
identity, writes a Keto role tuple, and issues a 24 h recovery code that is
sent by email. Role changes rewrite the Keto tuples under an advisory lock, so
the last super_admin can never be demoted.

## Authorization model (Keto OPL)

```mermaid
flowchart LR
  SA[super_admins] --> MA[manage_admins]
  SA --> MSC[manage_service_clients]
  SA --> MC[manage_customers]
  AD[admins] --> MC
  MC --> VC[view_customers]
  SU[supporters] --> VC
  MC --> VA[view_audit]
  MC --> RP[reveal_customer_pii]
```

Roles map to Keto relations on `Console:main`: `super_admin` → `super_admins`,
`admin` → `admins`, `support` → `supporters`. The subject is the subject set
`User:<uuid>#` (empty relation).

## Actors and entry points

| Endpoint | Keto permission | Extra |
| --- | --- | --- |
| `GET /admin/v1/admins` | `manage_admins` | cursor = Kratos page token |
| `POST /admin/v1/admins` | `manage_admins` | `Idempotency-Key` (uuid) required |
| `PUT /admin/v1/admins/{id}/role` | `manage_admins` | cannot change own role or demote last super_admin |
| CLI `identity-service admin bootstrap --email` | none (system) | first super_admin, prints link + code |

## Functional diagram — invite

```mermaid
flowchart TD
  A["POST /admin/v1/admins + Idempotency-Key"] --> G["Guard: admin, AAL2, CSRF, Keto manage_admins"]
  G --> N{"normalizeInvite: email, names, role"}
  N -- invalid --> E422[422]
  N -- ok --> R{"Reserve idempotency_key"}
  R -- "same key, different body" --> E409a[409 reused]
  R -- "same key, pending" --> E409b[409 in progress]
  R -- "same key, completed" --> REPLAY[201 stored response]
  R -- reserved --> K1["Kratos POST /admin/identities schema admin"]
  K1 -- "409 email exists" --> E409c["409 + release key"]
  K1 -- ok --> K2["Keto PATCH relation-tuples: insert role, delete others"]
  K2 --> K3["Kratos POST /admin/recovery/code expires_in 86400s"]
  K3 -- "any failure" --> COMP["compensate: Keto DELETE + Kratos DELETE, release key"]
  K3 -- ok --> TX["TX: profile + audit admin.invited + complete key"]
  TX -- fail --> COMP
  TX -- ok --> MAIL["SMTP SendInvitation (link + code)"]
  MAIL -- fail --> MF["compensate + audit admin.invitation_failed → 503"]
  MAIL -- ok --> OK["201 {id, email, role, invitation_expires_at}"]
```

## Sequence — invite

```mermaid
sequenceDiagram
  autonumber
  participant AW as Admin web
  participant IS as identity-service
  participant DB as identity DB
  participant KA as Kratos admin
  participant KW as Keto write
  participant ML as SMTP / Mailpit

  AW->>IS: POST /admin/v1/admins {email, name, role} + Idempotency-Key
  Note over IS: Guard: cookie session AAL2, AdminGate, Origin check, Keto manage_admins
  IS->>DB: INSERT idempotency_key(key, actor, sha256(body), 0) ON CONFLICT DO NOTHING
  IS->>KA: POST /admin/identities {schema_id:admin, state:active, traits:{email, name}}
  KA-->>IS: identity id
  IS->>KW: PATCH /admin/relation-tuples [insert Console:main#rel@User:id, delete other roles]
  IS->>KA: POST /admin/recovery/code {identity_id, expires_in:86400s}
  KA-->>IS: {recovery_link, recovery_code, expires_at}
  rect rgba(128,128,128,0.12)
    IS->>DB: INSERT profile (kind admin)
    IS->>DB: INSERT audit_event admin.invited {role}
    IS->>DB: UPDATE idempotency_key SET response_code 201, body
    IS->>DB: COMMIT
  end
  IS->>ML: SMTP "Your admin console invitation" (link, code, role, expiry)
  alt mail ok
    IS-->>AW: 201 {id, email, role, invitation_expires_at}
  else mail fails
    IS->>KW: DELETE relation-tuples for User:id
    IS->>KA: DELETE /admin/identities/{id}
    IS->>DB: audit admin.invitation_failed + release key
    IS-->>AW: 503 dependency_unavailable
  end
```

## Sequence — change role

```mermaid
sequenceDiagram
  autonumber
  participant AW as Admin web
  participant IS as identity-service
  participant KA as Kratos admin
  participant KR as Keto read
  participant KW as Keto write
  participant DB as identity DB

  AW->>IS: PUT /admin/v1/admins/{id}/role {role}
  IS->>IS: Guard (Keto manage_admins), target ≠ caller (else 403)
  IS->>KA: GET /admin/identities/{id} (404 if not admin)
  rect rgba(128,128,128,0.12)
    IS->>DB: BEGIN + pg_advisory_xact_lock
    IS->>KR: RolesOf(target) → previous_role
    opt demoting an active super_admin
      IS->>KR: all assignments + Kratos GET per super_admin
      Note over IS: ≤ 1 active super_admin → 409 last super_admin
    end
    IS->>DB: INSERT audit_event admin.role_changed {role, previous_role}
    IS->>KW: PATCH /admin/relation-tuples (insert new, delete others)
    IS->>DB: COMMIT
  end
  IS-->>AW: 200 Admin
```

## Sequence — list

```mermaid
sequenceDiagram
  autonumber
  participant AW as Admin web
  participant IS as identity-service
  participant KA as Kratos admin
  participant KR as Keto read
  AW->>IS: GET /admin/v1/admins?page_size=25&page_token=
  loop up to 5 Kratos pages until an admin is found
    IS->>KA: GET /admin/identities?page_size&page_token&include_credential=totp
  end
  IS->>KR: GET /relation-tuples?namespace=Console&object=main (all pages)
  IS-->>AW: 200 {items:[{id, email, name, role, state, mfa_enrolled}], next_page_token}
```

## Code references

- `deploy/ory/keto/namespaces.keto.ts`
- `services/identity-service/internal/adapter/httpapi/policy.go:35`
- `services/identity-service/internal/app/admins.go:103` — `List`, `:161` `Invite`, `:307` `provision`, `:352` `ChangeRole`
- `services/identity-service/internal/adapter/keto/keto.go:124` — check, `:239` patch
- `services/identity-service/internal/adapter/kratos/admin.go:122` — create, `:221` recovery code
- `services/identity-service/internal/adapter/mailer/smtp.go:73`
- `services/identity-service/internal/app/mutation.go:30` — `audited()`
- `services/identity-service/cmd/identity-service/main.go:691` — bootstrap CLI
- `apps/admin-web/src/features/admins/InviteAdminForm.tsx:22`, `AdminsPage.tsx:67`

## Related

[ADR-0005](../adr/0005-keto-for-authorization.md) ·
[03 §3.6](../architecture/03-auth-flows.md)
