# Requirements — Encrypted personal information

IDs are `PII-FR-xx` / `PII-NFR-xx`. Every requirement has a testable
acceptance criterion (AC).

## Data classification

| Field | Class | Stored as | Lookup |
| --- | --- | --- | --- |
| `phone_number` (E.164) | PII — confidential | AES-256-GCM ciphertext | HMAC-SHA256 blind index (equality) |
| `date_of_birth` (YYYY-MM-DD) | PII — confidential | ciphertext | — |
| `address` {line1, line2, city, region, postal_code, country} | PII — confidential | ciphertext (JSON) | — |
| `national_id` {type: cccd/passport/other, number} | PII — restricted | ciphertext (JSON) | — |
| `display_name`, `avatar_url`, `locale` | internal | plaintext (unchanged) | — |
| Kratos traits (email, name) | PII | Kratos DB (out of scope, volume encryption) | Kratos |

## Functional

| ID | Requirement | Acceptance criterion |
| --- | --- | --- |
| PII-FR-01 | A customer reads their own personal info (`GET /v1/me/personal-info`) | Returns decrypted values; empty object fields are `null` when never set; Bearer only |
| PII-FR-02 | A customer with a verified email replaces their personal info (`PUT /v1/me/personal-info`) | 200 with the stored values; unverified email → 403 `email_not_verified`; invalid field → 422 `validation_failed` with field codes and **no echoed values** |
| PII-FR-03 | Validation | phone normalised to E.164 `^\+[1-9]\d{7,14}$`; DOB a real date, age 13–120; country ISO-3166 alpha-2; national_id type ∈ {cccd, passport, other}, number `^[A-Z0-9]{6,20}$` (upper-cased); text ≤ 200 (line1/line2), ≤ 100 (city/region), ≤ 20 (postal), no control chars |
| PII-FR-04 | A customer erases their personal info (`DELETE /v1/me/personal-info`) | 204; the subject's data key is destroyed (crypto-shred); subsequent GET returns all `null`; audit event `customer.pii.erased` |
| PII-FR-05 | Admins see masked personal info (`GET /admin/v1/customers/{id}/personal-info`), permission `view_customers` | phone `+84*****789` (country code + last 3), DOB `1990-**-**`, address only city + country, national id type + last 3; target must be `customer` schema (else 404) |
| PII-FR-06 | Admins with `reveal_customer_pii` reveal full values (`POST /admin/v1/customers/{id}/personal-info/reveal` `{reason}`) | reason 10–200 chars required; supporters get 403; every reveal writes audit `customer.pii.revealed` with actor, target, fields and reason **before** the response is returned (audit failure → 500, nothing revealed) |
| PII-FR-07 | Admins look customers up by phone (`POST /admin/v1/customers/lookup` `{phone_number}`), permission `view_customers` | Uses the blind index only (no decryption scan); returns matching customer ids with masked info; phone sent in body, never in URL |
| PII-FR-08 | Customer erasure by an admin is out of scope for v1 | — |
| PII-FR-09 | Operator rotates the KEK and re-wraps data keys (`identity-service keys rewrap`) | After `rotate` + `rewrap`, all subject keys use the newest KEK version and every record still decrypts; command is idempotent and resumable |
| PII-FR-10 | Mobile: a "Personal information" screen (view, edit, erase with confirmation) | widget tests for form validation, save, erase; integration test against the stack |
| PII-FR-11 | Admin web: masked card on customer detail, Reveal with reason dialog (shown only with permission), phone lookup on the customers page | component tests with MSW; revealed values cleared when leaving the page, never cached in TanStack Query after unmount |
| PII-FR-12 | Local infra: `make up` starts Postgres, Kratos, Keto, Mailpit, OpenBao (+ init/unseal job) and identity-service; OpenBao data survives restart | `make down && make up` keeps previously stored PII readable |

## Non-functional

| ID | Requirement | Acceptance criterion |
| --- | --- | --- |
| PII-NFR-01 | Envelope encryption | One random 256-bit DEK per customer; DEK wrapped by the KEK in OpenBao Transit (`aes256-gcm96`); KEK never leaves OpenBao |
| PII-NFR-02 | Authenticated encryption with context binding | AES-256-GCM, random 96-bit nonce per write, AAD = `identity-service/customer_pii/<column>/<identity_id>/v1`; swapping a ciphertext to another row/column fails to decrypt (test) |
| PII-NFR-03 | Blind index | HMAC-SHA256 with a dedicated key distinct from the KEK, over the normalised value; the DB never holds the HMAC key |
| PII-NFR-04 | No plaintext PII at rest or in logs | Integration test: raw rows contain no plaintext; log capture during the PII e2e contains none of the test values; audit details hold field names, never values |
| PII-NFR-05 | Least privilege | identity-service's OpenBao token may only `encrypt`/`decrypt`/`rewrap` on the KEK and `hmac` on the index key; no `read`/`export`/`rotate`. Rotation uses a separate operator token |
| PII-NFR-06 | Fail closed | OpenBao unreachable → PII endpoints 503 `pii_unavailable`; other endpoints unaffected; no fallback to plaintext |
| PII-NFR-07 | Performance | Unwrapped DEKs cached in memory (TTL 5 min, bounded size, evicted on erase); p95 of `GET /v1/me/personal-info` < 50 ms locally with a warm cache |
| PII-NFR-08 | Compliance mapping | Docs map controls to GDPR Art. 5/17/25/32, Vietnam Decree 13/2023/ND-CP, OWASP ASVS V6/V8, NIST SP 800-38D/800-57 |
| PII-NFR-09 | Erasure in backups | Documented: wrapped DEKs in backups become unusable after backup retention; KEK is not destroyed per subject (accepted, "put beyond use") |
