# F11 — Customer management (admin)

Support staff list customers, look them up by login or phone, open a
customer's detail, and disable, enable, or revoke sessions. Lists show
**masked** login values only. Every mutation writes an audit row in the same
transaction as the Kratos call, before the call is made.

## Actors and entry points

| Endpoint | Keto permission | Audit |
| --- | --- | --- |
| `GET /admin/v1/customers?state=&page_size=&page_token=` | `view_customers` | — |
| `POST /admin/v1/customers/lookup` `{login}` or `{phone_number}` | `view_customers` | `customer.login.lookup` / `customer.pii.lookup` |
| `GET /admin/v1/customers/{id}` | `view_customers` | — |
| `POST /admin/v1/customers/{id}/disable` `{reason?}` | `manage_customers` | `customer.disabled` |
| `POST /admin/v1/customers/{id}/enable` `{reason?}` | `manage_customers` | `customer.enabled` |
| `DELETE /admin/v1/customers/{id}/sessions` | `manage_customers` | `customer.sessions_revoked` |

Every route first passes the admin Guard: cookie session, AAL2, AdminGate,
the CSRF check on mutations, and the Keto check
([F06](F06-request-authentication.md), [F09](F09-admin-sign-in.md)).

## Functional diagram

```mermaid
flowchart TD
  A[Admin action] --> G["Admin Guard + Keto permission"]
  G -- denied --> F403[403 forbidden]
  G --> T{Action}
  T -- list --> L1["Kratos list identities (≤ 5 pages), keep schema customer + state filter"]
  L1 --> L2["profiles + MaskMany: vault GetMany + OpenBao batch decrypt"]
  L2 --> L3["200 items with login.masked (login_unavailable if vault/OpenBao fails)"]
  T -- lookup --> K1{"login XOR phone_number?"}
  K1 -- no --> E422[422 body/one_of]
  K1 -- login --> LL["Lookup limiter 30/min, 200/day → HMAC lookup_key → vault → Kratos credentials_identifier=handle"]
  K1 -- phone --> LP["HMAC blind index → customer_pii by phone_bidx (≤ 20) → limiter per candidate → decrypt phone, constant-time compare"]
  LL --> LM["Mask login + PII per match"]
  LP --> LM
  LM --> LA["audit lookup event → 200 {items, truncated}"]
  T -- detail --> D1{"Kratos GET identity, schema customer?"}
  D1 -- no --> E404[404]
  D1 -- yes --> D2[200 Customer + masked login]
  T -- "disable / enable / revoke" --> M1{"customer exists?"}
  M1 -- no --> E404
  M1 -- yes --> M2["TX: audit row → Kratos PATCH state / DELETE sessions → COMMIT"]
  M2 -- "first Kratos call fails" --> RB["rollback audit → 503/500, nothing changed"]
  M2 -- "disable: PATCH ok, session revoke fails" --> PART["rollback audit, customer stays inactive → log unaudited_mutation, error"]
  M2 -- ok --> M3["Invalidate session cache (disable, revoke)"]
```

## Sequence — disable customer

```mermaid
sequenceDiagram
  autonumber
  participant AW as Admin web
  participant IS as identity-service
  participant KR as Keto read
  participant KA as Kratos admin
  participant DB as identity DB
  participant SC as SessionCache

  AW->>IS: POST /admin/v1/customers/{id}/disable {reason} (ReasonDialog requires it)
  IS->>KR: check Console:main#manage_customers@User:admin
  IS->>KA: GET /admin/identities/{id} (404 if missing or not customer)
  rect rgba(128,128,128,0.12)
    IS->>DB: BEGIN
    IS->>DB: INSERT audit_event customer.disabled {reason}
    IS->>KA: PATCH /admin/identities/{id} [replace /state inactive]
    IS->>KA: DELETE /admin/identities/{id}/sessions
    IS->>DB: COMMIT
  end
  IS->>SC: Invalidate(id)
  IS-->>AW: 200 {id, state:inactive}
  AW->>AW: invalidate customer, lists, audit queries
  Note over AW,IS: enable = PATCH state active only (audit customer.enabled, no revoke)<br/>revoke = DELETE sessions only (audit customer.sessions_revoked, 204)
```

## Sequence — lookup by login

```mermaid
sequenceDiagram
  autonumber
  participant AW as Admin web (LoginLookup)
  participant IS as identity-service
  participant OB as OpenBao transit
  participant DB as identity DB
  participant KA as Kratos admin

  AW->>IS: POST /admin/v1/customers/lookup {login:{type,value}}
  IS->>IS: Keto view_customers, LookupLimiter 1 unit (429)
  IS->>OB: transit/hmac/identity-login-pseudonym → lookup_key
  IS->>DB: SELECT login_identifier WHERE lookup_key
  IS->>KA: GET /admin/identities?credentials_identifier=handle
  opt legacy transition and email
    IS->>KA: GET /admin/identities?credentials_identifier=email
  end
  IS->>DB: GetMany login_identifier
  IS->>OB: transit/decrypt identity-login-kek (batch) → mask
  loop each match
    IS->>DB: customer_pii + subject_key
    IS->>OB: unwrap DEK (or cache) → open → Mask()
  end
  IS->>DB: INSERT audit_event customer.login.lookup {kind, matched_ids, matches}
  IS-->>AW: 200 {items:[{id, state, personal_info (masked), login (masked)}], truncated:false}
```

## Sequence — lookup by phone

```mermaid
sequenceDiagram
  autonumber
  participant AW as Admin web (PhoneLookup)
  participant IS as identity-service
  participant OB as OpenBao transit
  participant DB as identity DB
  participant KA as Kratos admin

  AW->>IS: POST /admin/v1/customers/lookup {phone_number}
  IS->>IS: Keto view_customers, normalise E.164 (422), LookupLimiter.Peek
  IS->>OB: transit/hmac/identity-pii-bidx {"phone_number:"+E164}
  IS->>DB: SELECT customer_pii WHERE phone_bidx AND bidx_key_version LIMIT 21
  IS->>IS: LookupLimiter.AllowN(candidates) (429)
  loop each candidate (≤ 20)
    IS->>OB: unwrap DEK (or cache)
    IS->>IS: open phone, constant-time compare (mismatch → skip)
    IS->>KA: GET /admin/identities/{id} (not customer → skip)
    IS->>IS: open remaining fields → Mask()
  end
  IS->>DB: INSERT audit_event customer.pii.lookup {bidx, matched_ids, matches, truncated}
  IS-->>AW: 200 {items:[{id, state, personal_info}], truncated}
```

## Errors and outcomes

| Condition | Response |
| --- | --- |
| Bad `page_size`, `state`, `page_token` | 422 |
| Lookup over quota | 429 `rate_limited` |
| Customer not found, or not a customer schema | 404 |
| Kratos or OpenBao down | 503 (the list degrades to `login_unavailable`) |
| Audit insert fails | 500, and nothing changes |

The server treats `reason` as optional (≤ 500 runes). Only the admin web makes
it mandatory for disable. Disable, enable and revoke take **no**
`Idempotency-Key`.

## Code references

- `services/identity-service/internal/app/customers.go:74` — `List`, `:108` `Get`, `:126` `Disable`, `:152` `Enable`, `:174` `Sessions`
- `services/identity-service/internal/app/personalinfo.go:427` — `LookupByPhone`, `:481` `LookupByLogin`
- `services/identity-service/internal/app/login.go:361` — `MaskMany`, `:419` `FindCustomers`
- `services/identity-service/internal/app/mutation.go:30` — `audited()`
- `services/identity-service/internal/adapter/httpapi/server.go:106` — list handler
- `apps/admin-web/src/features/customers/CustomersPage.tsx:31`, `CustomerDetailPage.tsx:114`, `LoginLookup.tsx:54`, `PhoneLookup.tsx:18`

## Related

[ADR-0013](../adr/0013-pseudonymous-customer-login-identifiers.md) ·
[10 §10.5](../architecture/10-pseudonymous-login.md)
