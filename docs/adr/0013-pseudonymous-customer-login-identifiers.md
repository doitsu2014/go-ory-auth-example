# 0013. Pseudonymous customer login identifiers in Kratos

- Status: Accepted; partly superseded by
  [ADR-0014](0014-customer-login-through-identity-service.md) (resolve
  endpoint, deterministic handle, device cache)
- Date: 2026-10-04

> **Note (ADR-0014).** The public resolve endpoint `POST /v1/auth/identifiers`
> is gone. Customers sign in, register and start recovery through
> identity-service (`POST /v1/auth/{login,registration,recovery}`). New Kratos
> handles are random, and the vault finds rows by a separate `lookup_key` =
> HMAC(address). The vault, AEAD, courier, pre-registration webhook and
> migration below still apply.

## Context

After ADR-0011, the Kratos database was the last store that held customer
contact data in plaintext: traits, credential identifiers,
verifiable/recovery addresses and courier messages (accepted risk,
06-security §6.7). A Kratos dump or backup exposed every customer's email,
and phone login would have added phone numbers to the same exposure.
Compliance (Vietnam PDPL 91/2025/QH15, Decree 13/2023) may require
field-level protection of customer contact data in every store.

Kratos needs an identifier to look up logins and an address to send codes
to. It has no trait encryption, and we never fork it or read its tables.
Passwords must never pass through identity-service.

## Decision

- Kratos stores only a **pseudonym** for customers:
  `login_id = base32(HMAC-SHA256(K, "login-id/v1" ‖ 0 ‖ kind ‖ 0 ‖ normalised value)) + "@login.invalid"`.
  K is the OpenBao Transit key `identity-login-pseudonym` (version pinned to
  1). `.invalid` is reserved (RFC 6761), so no MTA delivers to it, and Kratos
  accepts it as `format: email`. Email and phone use the same shape, so
  Kratos uses its email channel for both.
- **Resolve, then submit.** The app calls the public, IP-rate-limited
  `POST /v1/auth/identifiers` to get the pseudonym, then runs the Kratos
  native flow directly, with the pseudonym plus the password or code.
- The real address lives in identity-service's **login vault**
  (`login_identifier`): AEAD through Transit key `identity-login-kek`, with
  associated data = kind + pseudonym. Only `purpose: registration` writes to
  the vault. A pre-persist registration webhook (`response.parse: true`)
  rejects any `login_id` that has no vault entry, and any legacy `email`
  trait.
- **Courier.** Kratos `courier.delivery_strategy: http` posts to
  identity-service, with its own key. identity-service resolves pseudonym
  recipients, checks the identity in Kratos, and sends its own localised
  email (SMTP) or SMS (port; local sink in Mailpit). Delivery is
  de-duplicated with a keyed hash and has per-recipient quotas and an SMS
  budget. Admin mail passes through only to the admin's own address.
- The Kratos `profile` settings method is **disabled**, so `login_id` can
  never be changed by a customer.
- **Existing customers** are migrated by `identity-service pii
  migrate-kratos-logins`, which is crash-safe and idempotent. During the
  migration window Kratos uses a transition schema (`email` xor `login_id`).

Diagrams, pros/cons and future directions:
[10-pseudonymous-login](../architecture/10-pseudonymous-login.md).

## Alternatives considered

- **Body-rewriting proxy in front of Kratos.** Passwords would transit our
  code, and we would re-implement Kratos flow and CSRF handling. This is the
  option already rejected in 01-overview §1.5 (D).
- **Ciphertext trait plus HMAC identifier in Kratos.** The ciphertext would
  sit where our erasure ledger cannot shred it, there would be two traits to
  keep consistent, and we would still need courier resolution.
- **identity-service as the customers' OIDC IdP.** We would hold and verify
  passwords.
- **Disk/TDE only (option A).** Protects storage media, not a dump read by a
  database user. It remains the baseline under this ADR.
- **Per-customer DEK (ADR-0011) for the vault.** The address must be stored
  before the identity exists, it must survive a personal-info erase, and
  admin lists need batch decryption. This ADR therefore deviates from
  ADR-0011 for this table only.

## Consequences

- No customer email or phone number in any Kratos table (verified by the
  smoke test's SQL probe).
- Customer registration, sign-in and code delivery depend on identity-service
  and OpenBao. Admin sign-in does not.
- The resolve endpoint is an **oracle**: anyone who knows an address can
  learn its pseudonym. That is accepted, because the goal is data at rest in
  Kratos, not hiding account existence, and registration already reveals
  "taken". The endpoint is rate limited per IP (IPv6 per /64).
- Kratos's password-identifier similarity check compares against the
  pseudonym only. The app shows a hint instead.
- Loss of the HMAC key makes every customer unable to sign in. Back it up
  through OpenBao snapshots and escrowed unseal keys.
- Kratos courier messages are not removed by `kratos cleanup`, so an operator
  SQL script handles retention (`make kratos-scrub`).

## Revisit when

- Kratos supports trait encryption or pluggable identifier hashing.
- App attestation is available, to bind the resolve endpoint to genuine
  clients.
- The pseudonym key has to be rotated: run a re-key job that decrypts the
  vault, computes the v2 HMAC and runs the two-patch Kratos rewrite.
