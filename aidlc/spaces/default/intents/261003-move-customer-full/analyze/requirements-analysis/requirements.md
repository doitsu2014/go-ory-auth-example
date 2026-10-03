# Requirements — Customer name as encrypted PII

## Functional

| ID | Requirement | Acceptance criterion |
| --- | --- | --- |
| NAME-FR-01 | Kratos `customer` schema has no name | `traits` allows only `email`; native registration with `traits.name` → 400 from Kratos; `admin` schema unchanged |
| NAME-FR-02 | Name is a personal-info field | `PersonalInfo.name {first?, last?}` in `GET/PUT /v1/me/personal-info`; each part trimmed, ≤ 100 runes, no control chars (C0/C1, NUL); an object with both parts empty is treated as absent |
| NAME-FR-03 | Encrypted at rest like other PII | `customer_pii.name_ct` = AES-256-GCM under the customer's DEK, AAD binds format version, field `name`, identity_id (same scheme as the other columns); DB never holds the plaintext (raw-row check) |
| NAME-FR-04 | Masked for admins | `MaskedPersonalInfo.name` = first rune of each part + `***` (`{first:"A***", last:"N***"}`); support sees masked only; masked in lookup results too |
| NAME-FR-05 | Reveal | `fields` enum gains `name`; same permission (`reveal_customer_pii`), reason, rate limit and audit (`fields` recorded, never values) |
| NAME-FR-06 | Erase | `DELETE /v1/me/personal-info` crypto-shreds the name with the rest |
| NAME-FR-07 | Name no longer read from Kratos | `/v1/me` and admin customer list/detail stop returning `name` for customers (field deprecated in the contract, never populated); M2M responses unchanged (never had it) |
| NAME-FR-08 | Migration of existing names | `identity-service pii migrate-kratos-names [--dry-run]`: for each customer identity with `traits.name`: (a) if the customer has a `customer.pii.erased` audit event → only strip; (b) else if PII already has a name → only strip; (c) else store encrypted name (audit `customer.pii.name_migrated`, system actor, no values) and COMMIT, then strip from Kratos (read-compare-remove, DD5 — Kratos rejects JSON Patch `test`). Idempotent; prints counts; identity ids only in logs; non-zero exit if any identity failed |
| NAME-FR-09 | Clients | Mobile sign-up asks only email + password; personal-info screen (protected) edits and shows first/last name; the profile screen shows only the nickname. Admin web shows masked name in PersonalInfoCard / lookup, reveal can include it; the Kratos "name" row on the customer detail page is removed |
| NAME-FR-10 | Seeds and dev | `seed-customers` registers without `traits.name`, stores the name through `PUT /v1/me/personal-info`, sets a nickname display_name (`Khách hàng 01`…); `./dev up` runs the migration after the stack is healthy |

## Non-functional

| ID | Requirement | Acceptance criterion |
| --- | --- | --- |
| NAME-NFR-01 | No plaintext leaks | name never in logs, audit details, errors, metrics; types redact on fmt/slog/JSON like other PII types |
| NAME-NFR-02 | Backward-compatible data | Existing ciphertexts (no `name_ct`) keep decrypting; migration is additive (`name_ct` nullable) |
| NAME-NFR-03 | Safe migration ordering | Kratos value is removed only after the encrypted copy is committed; a crash between the two steps leaves the name in Kratos and a re-run completes it |
| NAME-NFR-04 | Tests | Unit: validation, mask, AAD binding; integration: PUT/GET/reveal/erase with name; migration cases (a)(b)(c) + idempotent re-run; smoke: raw row has no plaintext name and Kratos traits have no name |
| NAME-NFR-05 | Docs | 08-pii-protection classification, 04-data, 06-security (email-in-Kratos accepted risk with controls + owner), API guide, ADR-0011 amendment |
