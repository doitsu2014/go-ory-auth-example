# 0011. Envelope encryption with OpenBao Transit for customer PII

- Status: Proposed
- Date: 2026-10-03

## Context

Customers store their phone number, date of birth, address and national ID.
Plaintext columns would expose all of it to anyone with a DB dump, a backup,
or SQL access. The data also has to be searchable by phone, erasable on
request, and auditable when admins look at it.

## Decision

- Application-layer **envelope encryption**: one AES-256-GCM DEK per customer,
  wrapped by a KEK held in **OpenBao Transit**. The wrap is bound to the
  customer with `associated_data`.
- Per-field ciphertext with AAD = version ‖ column ‖ identity id.
- An HMAC **blind index**, computed by OpenBao, for phone lookup.
- **Crypto-shredding** for erasure, plus an erasure ledger that is re-applied
  after restores.
- **Masked by default**. Full reveal needs the Keto permit `reveal_customer_pii`,
  a reason code, and an audit row committed first.
- A `KeyManager` port. The `openbao` adapter is used in compose and production;
  `localkms` is used only in local and test environments.

## Alternatives considered

- **`pgcrypto` in SQL**: keys end up in SQL text, logs and `pg_stat_statements`.
- **Disk / TDE only**: does nothing against SQL-level access or dumps.
- **Transit `encrypt` per field**: one KMS call per field and no per-customer
  shredding.
- **Deterministic encryption for lookup**: leaks more and needs the key in the
  service.
- **Cloud KMS directly**: ties local development to a cloud account. It can
  still be added behind the port.

## Consequences

- OpenBao becomes a dependency of the PII endpoints only, and they fail closed.
- KEK rotation and re-wrap are operator tasks (`make kek-rotate`, `make keys-rewrap`).
- Equality on phone is the only search. Anything fuzzier needs a new design.
- Running OpenBao locally means a dev-only unseal key on a volume.

## Revisit when

- Moving to a managed KMS (AWS KMS / GCP KMS) in production.
- Kratos traits are minimised further (the email must stay for login).

## Amendment (2026-10-03, intent 261003-move-customer-full)

The customer name moved out of Kratos traits into this store as the encrypted
field `name` (`customer_pii.name_ct`, same DEK and AAD scheme). Existing names
are moved by `identity-service pii migrate-kratos-names` (08 §8.11). The
customer email is the only customer PII left in plaintext, in Kratos
(accepted risk, 06-security §6.7).
