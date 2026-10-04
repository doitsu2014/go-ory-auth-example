# Intent Statement — Pseudonymous customer login identifiers in Kratos

## Problem

Customer login identifiers (email today, phone number next) are stored in
plaintext in the Kratos database: `identities.traits`,
`identity_credential_identifiers`, `identity_verifiable_addresses`,
`identity_recovery_addresses` and courier messages. Every other customer PII
field is already envelope-encrypted in identity-service (ADR-0011), so the
Kratos database is the last store where a dump, backup or over-privileged SQL
session discloses customer contact data. This is recorded as an accepted risk
(06-security §6.7) whose revisit trigger is *"login by a pseudonymous
identifier is acceptable"*. The owner has now chosen that route (option B) and
asked for it to be done properly.

## Users

| User | Client | Needs |
| --- | --- | --- |
| Customer | Flutter mobile app | Register, sign in, verify, recover and change their login identifier using a real **email or phone number**, with no visible change other than choosing email/phone |
| Admin (supporter/admin/super_admin) | React admin web | Find a customer by email/phone and see a masked contact; reveal under the existing permission + reason + audit |
| Operator | CLI / compose | Migrate existing customers once, rotate keys, run cleanup; SMS adapter configurable |
| Auditor / DPO | DB + audit | Show that no store outside identity-service's encrypted vault holds a customer contact address |

## Success criteria (observable)

1. After registration and after migration, a raw `SELECT` on every Kratos
   table (`identities`, `identity_credential_identifiers`,
   `identity_verifiable_addresses`, `identity_recovery_addresses`,
   `courier_messages`) shows **no customer email or phone number**, only
   pseudonyms of the form `<base32>@<reserved .invalid domain>`.
2. The real address exists only as AEAD ciphertext plus a keyed blind index in
   the identity database, under keys held in OpenBao Transit.
3. Passwords and codes still go **only** from the client to Kratos public; no
   identity-service endpoint receives a password.
4. Verification and recovery codes reach the customer's real mailbox (email)
   or phone (SMS) through identity-service, delivered from Kratos's `http`
   courier; Kratos never talks SMTP/SMS for customers.
5. Mobile end-to-end: register → verify → sign out → sign in → forgot
   password → change password all work for both email and phone. (Changing
   the login identifier itself is not offered by the app today and stays a
   follow-up; the server side must still keep it safe if attempted.)
6. Admin customer search by exact email/phone and the masked/revealed contact
   view keep working; the audit log never stores the address.
7. A migration CLI converts existing customers idempotently; a crash at any
   step leaves the customer able to sign in.
8. Pseudonym endpoint and courier webhook are rate-limited, never log the
   address, and fail closed when OpenBao is unavailable.

## In scope

- Pseudonym scheme: `HMAC-SHA256` via OpenBao Transit (dedicated key, pinned
  version) over a typed, normalised identifier (email lower-cased / phone
  E.164), encoded into a Kratos-valid, undeliverable email-shaped string.
- Encrypted contact vault in identity-service (ciphertext + blind index), and
  the pseudonym resolution endpoint used by the mobile app before every
  Kratos flow that takes an identifier.
- Kratos `customer` schema change, courier `http` delivery to an
  identity-service webhook that resolves and sends email (SMTP) or SMS
  (adapter port; local dev sink).
- Migration CLI for existing customers; erasure on account deletion;
  cleanup of unbound vault entries.
- Mobile UI (email/phone choice), admin web (search + masked contact), API
  contract, docs, ADR, accepted-risk update, tests.

## Out of scope

- Admin identities (employee work email stays in the `admin` schema; browser
  flows; invitation mail unchanged).
- Passwordless login (code as first factor stays disabled).
- A production SMS provider integration beyond the adapter port + a local sink.
- Kratos session IP / user agent (separate accepted risk).
- Storage-level encryption (TDE) — already the baseline, not this intent.

## Constraints

- Standing rules: passwords never transit identity-service; no secret in the
  app; Kratos is never forked or read via SQL by our code; Kratos admin API
  stays private; contract-first API (ADR-0010); hexagonal Go service
  (ADR-0009); secrets never in source or logs (org memory).
- Kratos pinned at v26.2.0; behaviour of `http` courier, identifier
  normalisation and pre-persist webhooks must be verified on that version.
- Regulatory: Vietnam PDPL 91/2025/QH15 / Decree 13/2023; erasure deadlines
  as in 08-pii §8.5.

## Assumptions

- The pseudonym resolution endpoint is a deliberate, rate-limited oracle:
  someone who already knows an address can learn its pseudonym. The goal is
  protection of data **at rest in Kratos**, not hiding account existence
  (registration already reveals "address taken" — accepted risk §6.7).
- One login identifier per customer (email **or** phone) in this intent.
- A login phone is a different field from the self-declared
  `personal_info.phone_number`; they are not merged.
