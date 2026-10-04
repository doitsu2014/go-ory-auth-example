# F08 — Customer personal info (PII)

The customer reads, replaces, and erases their own name, phone number, date of
birth, address, and national ID. Each field is sealed with **AES-256-GCM**
under a per-customer **DEK**. OpenBao Transit (`identity-pii-kek`) wraps the
DEK. The phone number also gets an HMAC **blind index**, which admin lookup
uses ([F12](F12-admin-pii-access.md)). Erasing deletes the DEK, so the data
can no longer be decrypted (crypto-shredding).

## Actors and entry points

| Actor | Client | Endpoint | Gate |
| --- | --- | --- | --- |
| Customer | `PersonalInfoScreen` | `GET /v1/me/personal-info` | customer |
| Customer | `PersonalInfoScreen` save | `PUT /v1/me/personal-info` (full replace) | + verified login |
| Customer | `PersonalInfoScreen` erase | `DELETE /v1/me/personal-info` | customer (no verification needed) |

## Functional diagram

```mermaid
flowchart TD
  A["/v1/me/personal-info"] --> G["Guard (F06) + kind = customer"]
  G --> M{Method}
  M -- GET --> R1{"customer_pii row?"}
  R1 -- no --> R0["200 all null"]
  R1 -- yes --> R2{"subject_key present and key_id matches?"}
  R2 -- no --> E500[500 internal]
  R2 -- yes --> R3["DEK from cache, else OpenBao unwrap"]
  R3 --> R4["GCM open each column"]
  R4 -- fail --> E500
  R4 -- ok --> R5["200 PersonalInfo"]

  M -- PUT --> P1{"EmailVerified?"}
  P1 -- no --> NV[403 email_not_verified]
  P1 -- yes --> P2{"strict body + Normalize + Validate"}
  P2 -- invalid --> E422[422 validation_failed]
  P2 -- ok --> P3{"Phone given?"}
  P3 -- yes --> P4["OpenBao HMAC blind index"]
  P3 -- no --> P5
  P4 --> P5{"subject_key exists?"}
  P5 -- yes --> P6[Unwrap existing DEK]
  P5 -- no --> P7["NewDEK → OpenBao wrap → INSERT subject_key ON CONFLICT"]
  P6 --> P8["Seal each field, AAD = prefix ‖ field ‖ identity_id"]
  P7 --> P8
  P8 --> P9["TX: UPSERT customer_pii + audit customer.pii.updated"]
  P9 -- "conflict (concurrent erase)" --> P10[Evict DEK, retry once]
  P10 --> P5
  P9 -- ok --> P11["200 normalised input + updated_at"]

  M -- DELETE --> D1["Read legacy Kratos name trait (error kept, not fatal yet)"]
  D1 --> D2["TX: DELETE subject_key (cascades customer_pii)"]
  D2 --> D3{"Anything deleted or legacy name?"}
  D3 -- no --> D3n["commit, no audit"]
  D3 -- yes --> D4["audit customer.pii.erased, commit, evict DEK (tombstone)"]
  D3n --> D5
  D4 --> D5{"Kratos lookup failed earlier?"}
  D5 -- yes --> E503["503 erase incomplete (data already shredded)"]
  D5 -- no --> D6{"Legacy name present?"}
  D6 -- yes --> D7["Kratos PATCH remove /traits/name"]
  D6 -- no --> D204b[204]
  D7 -- ok --> D204b
  D7 -- "fails (not 404)" --> E503
```

## Sequence — PUT (first write)

```mermaid
sequenceDiagram
  autonumber
  participant MA as Mobile app
  participant IS as identity-service
  participant OB as OpenBao transit
  participant DB as identity DB

  MA->>IS: PUT /v1/me/personal-info {name, phone_number, date_of_birth, address, national_id}
  IS->>IS: Guard, EmailVerified (403), strict body + Validate (422)
  opt phone present
    IS->>OB: POST transit/hmac/identity-pii-bidx/sha2-256 {"phone_number:"+E164, key_version 1}
    OB-->>IS: phone_bidx
  end
  IS->>DB: SELECT subject_key WHERE identity_id
  alt no key yet
    IS->>IS: DEK = 32 random bytes, key_id = uuid
    IS->>OB: POST transit/encrypt/identity-pii-kek {DEK, AD identity_id/key_id}
    OB-->>IS: wrapped_dek vault:vN:...
    IS->>DB: INSERT subject_key ON CONFLICT (identity_id) DO NOTHING RETURNING
    Note over IS: lost race → re-read winner's key and unwrap it
  else key exists
    IS->>IS: DEKCache.Get(key_id) or OpenBao transit/decrypt
  end
  Note over IS: Seal each field: 0x01 ‖ nonce ‖ ct ‖ tag
  rect rgba(128,128,128,0.12)
    IS->>DB: BEGIN
    IS->>DB: INSERT customer_pii ... ON CONFLICT DO UPDATE (ct columns, phone_bidx)
    IS->>DB: INSERT audit_event customer.pii.updated {fields:[names]}
    IS->>DB: COMMIT
  end
  IS-->>MA: 200 PersonalInfo + updated_at
```

## Sequence — DELETE (crypto-shred)

```mermaid
sequenceDiagram
  autonumber
  participant MA as Mobile app
  participant IS as identity-service
  participant KA as Kratos admin
  participant DB as identity DB

  MA->>IS: DELETE /v1/me/personal-info
  IS->>KA: GET /admin/identities/{id} (legacy traits.name)
  rect rgba(128,128,128,0.12)
    IS->>DB: DELETE subject_key WHERE identity_id RETURNING key_id
    Note over DB: FK cascade deletes customer_pii
    IS->>DB: INSERT audit_event customer.pii.erased (only if something was erased)
    IS->>DB: COMMIT
  end
  IS->>IS: DEKCache.Evict(key_id) leaves a 2×TTL tombstone
  alt legacy name present
    IS->>KA: PATCH /admin/identities/{id} [remove /traits/name]
  end
  IS-->>MA: 204 (or 503 if the Kratos part failed)
```

## Side effects

| Table / system | GET | PUT | DELETE |
| --- | --- | --- | --- |
| `subject_key` | read | insert, first write only | delete |
| `customer_pii` | read | upsert | cascade delete |
| `audit_event` | — | `customer.pii.updated` | `customer.pii.erased` |
| OpenBao | decrypt DEK | HMAC, encrypt or decrypt DEK | — |
| Kratos admin | — | — | read, then remove legacy name |

`identity_app` may update only the listed columns (migrations 0004 and 0005).
It cannot delete `customer_pii` directly.

## Code references

- `apps/mobile/lib/features/profile/presentation/personal_info_screen.dart:188` — save, `:226` erase
- `services/identity-service/internal/adapter/httpapi/personalinfo.go:17`
- `services/identity-service/internal/app/personalinfo.go:123` — `GetMine`, `:133` `PutMine`, `:235` `EraseMine`, `:689` `keyForCreated`
- `services/identity-service/internal/app/dekcache.go` — LRU, TTL 5 m, tombstones
- `services/identity-service/internal/crypto/envelope/envelope.go:46` — DEK, seal, AAD
- `services/identity-service/internal/adapter/openbao/openbao.go:306` — wrap, unwrap, HMAC
- `services/identity-service/db/queries/pii.sql`
- `services/identity-service/db/migrations/0003_customer_pii.sql`, `0004_pii_column_grants.sql`, `0005_customer_pii_name.sql`

## Related

[ADR-0011](../adr/0011-envelope-encryption-for-pii.md) ·
[08 PII protection](../architecture/08-pii-protection.md)
