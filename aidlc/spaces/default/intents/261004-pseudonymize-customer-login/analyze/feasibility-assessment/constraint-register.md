# Constraint & Risk Register — Pseudonymous customer login identifiers

## Hard constraints

| ID | Constraint | Source | Consequence for design |
| --- | --- | --- | --- |
| HC-01 | Passwords / codes never transit identity-service | 02-components §2.3 | Resolve-then-submit; no body-rewriting proxy |
| HC-02 | No secret in the mobile app | §2.5 | HMAC key only in OpenBao |
| HC-03 | identity-service never reads/writes Kratos tables | §2.1 | Scrub is an operator SQL script with the Kratos role, not service code |
| HC-04 | Kratos v26.2.0 pinned | 04-data / compose | Behaviours verified in spikes S1–S5 |
| HC-05 | Contract-first API, RFC 9457 | ADR-0010 | OpenAPI updated before handlers |
| HC-06 | No PII in URLs, logs, audit | 08-pii §8.9 | Lookup by body; logger redaction keys |
| HC-07 | Erasure deadline 72 h (Decree 13 Art. 16) / PDPL | 08-pii §8.5 | Vault entry erased with the account |
| HC-08 | Courier payload carries no message id | spike S2 | Derived dedupe key |
| HC-09 | Single admin PATCH loses verification | spike S4b | Two-patch migration with `legacy_verified` |
| HC-10 | `kratos cleanup` never removes courier messages | spike S5 | Scrub/retention SQL job |

## Risks

| ID | Risk | L | I | Mitigation | Owner |
| --- | --- | --- | --- | --- | --- |
| R-01 | identity-service or OpenBao down ⇒ customers cannot register / sign in / receive codes | M | H | Resolve is cheap and stateless except registration; HA identity-service; Kratos courier retries; readiness alert on OpenBao; admins unaffected for login (no resolve) | platform lead |
| R-02 | Resolve endpoint used as an existence/enumeration oracle | M | L | No existence signal in response; constant code path; per-IP limits; registration already reveals "taken" (accepted risk §6.7) | identity-service maintainers |
| R-03 | HMAC key loss ⇒ nobody can sign in (pseudonyms not recomputable) | L | Critical | Key in OpenBao with backups; `deletion_allowed=false`; documented key-backup procedure; version pinned, no auto-rotate | operator |
| R-04 | Old app builds send plaintext identifiers; plaintext lingers in failed flows | M | M | Schema rejects plaintext; minimum app version enforced before rollout; scheduled `kratos cleanup sql --keep-last 24h` | mobile lead |
| R-05 | Migration crash mid-identity | L | M | Idempotent, two-patch with `legacy_verified`; dry-run; per-identity audit | identity-service maintainers |
| R-06 | Kratos `identifier_similarity_check` no longer sees the real address | H | L | Accept; HIBP + min length 12 remain. Mobile adds a client-side "password must not contain your email/phone" hint (no server secret involved) | mobile lead |
| R-07 | Courier dispatcher sends a duplicate on Kratos retry | M | L | Dedupe key 24 h | identity-service maintainers |
| R-08 | Mail/SMS provider outage | M | M | Non-2xx back to Kratos ⇒ Kratos retries (`message_retries`); metric + alert | operator |
| R-09 | Admin mail regression via the global http courier | L | M | Dispatcher passes non-pseudonym recipients through; existing admin integration suite must stay green | identity-service maintainers |
| R-10 | Pseudonym rotation impossible without re-keying every identity | L | M | Out of scope; documented: rotation = run the migration CLI against a new key version (each login re-resolved) | — |

## Backwards compatibility

- `Me.email` becomes optional; `login` added. First-party clients only.
- `GET /admin/v1/customers?email=` removed in favour of body lookup.
- Kratos schema id stays `customer`; trait name changes `email` → `login_id`.
