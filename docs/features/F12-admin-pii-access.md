# F12 — Admin PII access (masked view and reveal)

The customer detail page shows a **masked** personal-info card. An admin with
`reveal_customer_pii` can reveal chosen fields in plain text. The admin must
give a reason code, and may add a ticket reference. Each reveal is rate
limited (20/hour per admin) and **audited before any plaintext leaves the
service**. The SPA hides the revealed values after 60 s.

## Actors and entry points

| Endpoint | Keto | Limiter | Audit |
| --- | --- | --- | --- |
| `GET /admin/v1/customers/{id}/personal-info` | `view_customers` | 300/min per admin | — |
| `POST /admin/v1/customers/{id}/personal-info/reveal` `{reason_code, ticket_ref?, fields?}` | `reveal_customer_pii` | 20/hour per admin | `customer.pii.revealed` |

`reason_code` ∈ `customer_support_request`, `identity_verification`,
`fraud_investigation`, `legal_request`. `fields` ⊆ `name`, `phone_number`,
`date_of_birth`, `address`, `national_id`, `login`. Omitting `fields` reveals
all of them.

## Functional diagram

```mermaid
flowchart TD
  A[Reveal clicked] --> B{"useCan reveal_customer_pii?"}
  B -- no --> HID[Button hidden]
  B -- yes --> C["RevealDialog: reason_code, ticket_ref regex, fields"]
  C --> D["POST .../personal-info/reveal"]
  D --> G["Admin Guard: AAL2, CSRF, Keto reveal_customer_pii"]
  G -- denied --> F403[403]
  G --> V{"strict body + validate reason/fields"}
  V -- invalid --> E422[422]
  V -- ok --> RL{"RevealLimiter 20/h"}
  RL -- exceeded --> E429[429]
  RL -- ok --> K{"Kratos: is customer?"}
  K -- no --> E404[404]
  K -- yes --> P{"PII fields requested?"}
  P -- yes --> P1["customer_pii + subject_key → DEK unwrap → GCM open requested columns"]
  P -- no --> L
  P1 --> L{"login requested?"}
  L -- yes --> L1["vault by pseudonym → OpenBao decrypt login"]
  L -- no --> AU
  L1 --> AU["Audit customer.pii.revealed {fields, reason_code, ticket_ref}"]
  AU -- fails --> E500["500, buffers zeroed, nothing returned"]
  AU -- ok --> OK[200 RevealedPersonalInfo]
  OK --> T["SPA shows plaintext, hides after 60 s or on unmount"]
```

## Sequence — reveal

```mermaid
sequenceDiagram
  autonumber
  participant AW as Admin web (RevealDialog)
  participant IS as identity-service
  participant KR as Keto read
  participant KA as Kratos admin
  participant DB as identity DB
  participant OB as OpenBao transit

  AW->>IS: POST /admin/v1/customers/{id}/personal-info/reveal {reason_code, ticket_ref, fields}
  IS->>KR: check Console:main#reveal_customer_pii@User:admin
  IS->>IS: validateReveal (422), RevealLimiter 20/h (429)
  IS->>KA: GET /admin/identities/{id} (404 if not customer)
  opt PII fields requested
    IS->>DB: SELECT customer_pii, subject_key
    IS->>OB: POST transit/decrypt/identity-pii-kek {wrapped_dek, AD identity_id/key_id} (or DEKCache)
    IS->>IS: AES-GCM open requested columns only
  end
  opt login requested
    IS->>DB: SELECT login_identifier WHERE pseudonym = handle
    IS->>OB: transit/decrypt identity-login-kek
  end
  IS->>DB: INSERT audit_event customer.pii.revealed {fields, reason_code, ticket_ref} (own TX)
  alt audit ok
    IS-->>AW: 200 {name, phone_number, ..., login:{type,value}, updated_at}
    AW->>AW: show for 60 s, invalidate audit queries
  else audit fails
    IS-->>AW: 500 (plaintext discarded)
  end
```

## Sequence — masked view

```mermaid
sequenceDiagram
  autonumber
  participant AW as Admin web (PersonalInfoCard)
  participant IS as identity-service
  participant KA as Kratos admin
  participant DB as identity DB
  participant OB as OpenBao transit
  AW->>IS: GET /admin/v1/customers/{id}/personal-info
  IS->>IS: Keto view_customers, MaskedLimiter 300/min
  IS->>KA: GET /admin/identities/{id}
  IS->>DB: SELECT customer_pii, subject_key (key_id must match)
  IS->>OB: unwrap DEK (or cache)
  IS->>IS: open all, Info.Mask()
  IS-->>AW: 200 {has_*, masked name/phone/dob, address city+country, national_id masked}
```

## Code references

- `services/identity-service/internal/app/personalinfo.go:302` — `GetMasked`, `:324` `Reveal`, `:390` `revealFields`
- `services/identity-service/internal/app/login.go:345` — login `Reveal`
- `services/identity-service/internal/app/ratelimit.go:21` — lookup / reveal / masked limits
- `services/identity-service/internal/adapter/httpapi/personalinfo.go:113`, `:200` `toRevealed`
- `apps/admin-web/src/features/customers/PersonalInfoCard.tsx:111`, `RevealDialog.tsx:21`

## Related

[ADR-0011](../adr/0011-envelope-encryption-for-pii.md) ·
[08 §8.4](../architecture/08-pii-protection.md)
