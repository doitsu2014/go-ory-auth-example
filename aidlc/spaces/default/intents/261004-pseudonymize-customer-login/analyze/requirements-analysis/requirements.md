# Requirements — Pseudonymous customer login identifiers

IDs: `PLI-FR-xx` (functional), `PLI-NFR-xx` (non-functional), `PLI-C-xx`
(constraint), `PLI-A-xx` (assumption). Priority is MoSCoW. Every requirement
traces to a success criterion (SC-n) of the intent statement and has a
testable acceptance criterion (AC).

## Glossary

| Term | Meaning |
| --- | --- |
| login identifier | What the customer types: an email address or a phone number |
| typed identifier | `(type, normalised value)`; type ∈ {`email`, `phone`}; email lower-cased + trimmed, phone E.164 |
| pseudonym | `<base32(HMAC-SHA256(K_p, "login-id/v1" ‖ 0x00 ‖ type ‖ 0x00 ‖ value))>@<pseudonym domain>`; the only form Kratos ever stores |
| vault | identity-service table holding `pseudonym → AEAD(ciphertext of value)` + type |
| bound | a vault entry whose pseudonym is the `login_id` of an existing Kratos identity |

## Functional

| ID | Pri | Requirement | AC | Verify | SC |
| --- | --- | --- | --- | --- | --- |
| PLI-FR-01 | M | **Resolve**: `POST /v1/auth/identifiers {type, value, purpose}` returns `{identifier: <pseudonym>}`; unauthenticated; purpose ∈ {`registration`, `sign_in`, `recovery`, `verification`} | Same input → same pseudonym; email case/whitespace variants → same pseudonym; email and phone with same digits → different pseudonyms; invalid value → 422 `validation_failed` with field codes and **no echoed value**; response identical in shape and status whether or not an account exists | unit + integration | 1,3 |
| PLI-FR-02 | M | Only `purpose: registration` persists: encrypt the value and insert the vault entry if absent (idempotent) | After resolve(registration) a vault row exists with ciphertext; after resolve(sign_in) of a new address no row exists; repeated registration resolve keeps one row | integration | 2 |
| PLI-FR-03 | M | Kratos `customer` schema v2: single trait `login_id`, `format: email`, `pattern` restricted to the pseudonym shape, password identifier, verification + recovery `via: email` | Registration with a plaintext email in `login_id` → 400 from Kratos (pattern); with a pseudonym → 200 | integration (Kratos) | 1 |
| PLI-FR-04 | S | Pre-persist registration check: registration whose `login_id` has no vault entry is rejected with a flow error on the `traits.login_id` node | Registering a well-shaped but unknown pseudonym → 400 flow with message id; known pseudonym → 200 | integration | 1,8 |
| PLI-FR-05 | M | **Courier dispatch**: Kratos courier uses `http` delivery to `POST /internal/hooks/kratos/courier` (webhook port, API key). For a pseudonym recipient the service resolves the vault entry and sends via email (SMTP) or SMS by entry type; for any other recipient (admins) it sends the email unchanged | Customer email registration → verification code arrives in Mailpit at the real address; phone registration → SMS sink receives the code; admin recovery mail still arrives; a pseudonym with no vault entry → non-2xx + metric `courier_unresolved_total`, nothing sent | integration + e2e | 4 |
| PLI-FR-06 | M | Dispatcher renders its own localised (vi/en) messages per Kratos template type (verification code, recovery code, and their "invalid/unknown" variants); never forwards Kratos-rendered bodies that might contain the pseudonym | Snapshot tests per template type and locale; message bodies contain the code and no pseudonym | unit | 4 |
| PLI-FR-07 | M | `GET /v1/me` returns `login: {type, value}` (decrypted, owner only) and `email` only when `type = email` (deprecated); `email_verified` keeps meaning "login identifier verified" | Phone user → `login.type = phone`, no `email`; email user → both; vault unavailable → 503 `dependency_unavailable` for `/v1/me` only | integration | 5 |
| PLI-FR-08 | M | Mobile: sign up / sign in / forgot password / verify let the customer choose **email or phone**, resolve the pseudonym first, then run the Kratos native flow with the pseudonym. Displayed contact comes from `/v1/me`, never from session traits | Widget tests per screen for both types; integration test against the stack: register → verify → sign out → sign in → recover, for email and phone | widget + integration | 5 |
| PLI-FR-09 | M | Mobile re-authentication (settings privileged session) uses `session.identity.traits.login_id` (already a pseudonym) | Password change after 15 min works without re-typing the identifier | widget | 5 |
| PLI-FR-10 | M | Admin list/detail show a **masked** login contact (`login: {type, masked}`) obtained by batch decryption; one key-manager round trip per page | email `a***@e***.com` (first char of local part and of domain label, TLD kept); phone `+84*******567`; list of 25 customers → exactly one batch decrypt call | unit + integration | 6 |
| PLI-FR-11 | M | Admin lookup by login contact in the body: `POST /admin/v1/customers/lookup {login: {type, value}}`, permission `view_customers`; `?email=` on `GET /admin/v1/customers` is removed | Exact match returns the customer; non-customer schema never returned; URL carries no contact; old `?email=` → 400 | integration + web component test | 6 |
| PLI-FR-12 | M | Admin reveal includes the login contact as field `login`, same permission/reason/audit as other PII fields | Reveal with `fields:["login"]` returns full value; audit details list `login`, never the value | integration | 6 |
| PLI-FR-13 | M | **Migration CLI** `identity-service pii migrate-kratos-logins [--dry-run]`: for each customer identity whose `login_id`/`email` is not a pseudonym: store bound vault entry, then rewrite Kratos traits (+ credential identifier + verifiable/recovery address) to the pseudonym **preserving verification status**; idempotent, resumable, read-compare-write | Fixture of 3 legacy customers (verified, unverified, mid-migration crash) → all sign in with their real email afterwards; verified stays verified; second run changes nothing; `--dry-run` writes nothing | integration | 7 |
| PLI-FR-14 | M | Post-migration scrub: documented and scripted removal of residual plaintext in Kratos (old self-service flows, courier messages, verification/recovery codes) | After migrate + scrub, SQL probe over Kratos tables finds no test address | integration (SQL probe) | 1,7 |
| PLI-FR-15 | M | Unbound purge: `identity-service pii purge-unbound-logins` deletes vault entries older than 24 h that are not bound and for which Kratos admin finds no identity with that credential identifier | Unbound > 24 h without identity → deleted; unbound with identity (webhook missed) → marked bound, kept | integration | 2 |
| PLI-FR-16 | M | Binding: after-registration webhook marks the vault entry bound (identity id); lazy bind on first `/v1/me` | After registration row is bound to the identity id; webhook down → bound on `/v1/me` | integration | 2 |
| PLI-FR-17 | S | Account deletion hook point: when a customer identity is deleted (admin action or future self-service), its vault entry is deleted in the same use case and audited `customer.login.erased` | Service-level test with a fake Kratos | unit | 2 |
| PLI-FR-18 | C | In-app change of login identifier | Not built; server side: a settings change to an unknown pseudonym yields no delivery and stays unverified | — | — |
| PLI-FR-19 | M | Docs + ADR: ADR-0013 (pseudonymous login identifiers), 08-pii (§8.1, §8.10 rewritten), 03-auth-flows (sequence diagrams with resolve step), 06-security §6.7 risk replaced, 01-overview non-goal updated, runbook for migration order | Docs reviewed in code review; links valid | review | all |

## Non-functional

| ID | Pri | Requirement | AC | Verify | SC |
| --- | --- | --- | --- | --- | --- |
| PLI-NFR-01 | M | No customer contact in Kratos: SQL probe over `identities`, `identity_credential_identifiers`, `identity_verifiable_addresses`, `identity_recovery_addresses`, `courier_messages`, `selfservice_*_flows` after the e2e suite finds none of the test addresses | SQL probe test in the integration suite | integration | 1 |
| PLI-NFR-02 | M | Key separation: pseudonym HMAC key `identity-login-pseudonym` is a dedicated Transit HMAC key (pinned version, no auto-rotate); contact values are encrypted with a key other than the HMAC key; neither key is exportable | OpenBao policy test: app token can `hmac` on pseudonym key, `encrypt/decrypt` on the vault key, nothing else new | integration | 2 |
| PLI-NFR-03 | M | AEAD binding: vault ciphertext is bound to its pseudonym and type (AAD), so a ciphertext copied to another row fails to decrypt | Unit test swaps ciphertexts between rows → decrypt error | unit | 2 |
| PLI-NFR-04 | M | Passwords and codes never reach identity-service: no new endpoint accepts `password`/`code`; resolve endpoint rejects unknown body fields | Contract test: unknown field → 400; router test: no handler binds a password field | unit | 3 |
| PLI-NFR-05 | M | Abuse limits: resolve 20/min and 200/day per client IP (+ global circuit), registration-purpose 5/min per IP; courier webhook only on the webhook port with API key | Exceed → 429 `rate_limited` with `Retry-After`; courier on public port → 404 | unit + integration | 8 |
| PLI-NFR-06 | M | Fail closed: OpenBao unavailable → resolve 503 `dependency_unavailable`; courier webhook non-2xx (Kratos retries); nothing falls back to plaintext | Fault-injection test | integration | 8 |
| PLI-NFR-07 | M | No contact in logs/metrics/audit: logger redacts `value`, `identifier`, `login`, `recipient`; audit details hold type + pseudonym hash prefix at most | Log-capture test during e2e finds no test address or pseudonym | integration | 8 |
| PLI-NFR-08 | M | Enumeration parity: resolve latency/shape does not depend on account existence beyond noise (always compute HMAC, registration always encrypts) | Unit test: same code path for existing/new entries; no existence field in response | unit | 8 |
| PLI-NFR-09 | S | Performance: resolve p95 < 60 ms locally; `/v1/me` p95 < 80 ms with warm caches; admin list page adds ≤ 1 key-manager call | Benchmark/integration timing logged in launch | integration | — |
| PLI-NFR-10 | M | Courier reliability: dispatcher idempotent per Kratos message id (dedupe for 24 h) so Kratos retries never send twice | Two identical webhook calls → one delivery | unit | 4 |
| PLI-NFR-11 | M | Compatibility: admins unaffected (schema, login, invitation, recovery mail); population guard webhooks unchanged | Existing admin e2e/integration suites pass unchanged | regression | 6 |
| PLI-NFR-12 | S | Mobile minimum version: server rejects registration of plaintext identifiers (PLI-FR-03), so old app builds fail registration with a Kratos validation message; rollout order documented | Runbook in docs | review | 7 |

## Constraints

| ID | Constraint | Source |
| --- | --- | --- |
| PLI-C-01 | Passwords never transit identity-service; no body-rewriting proxy | 02-components §2.3, 01-overview §1.5 D |
| PLI-C-02 | No secret in the mobile app | 02-components §2.5 |
| PLI-C-03 | Kratos not forked; no SQL reads of Kratos tables by our code (the SQL probe is a **test** utility only) | 02-components §2.1 |
| PLI-C-04 | Contract-first OpenAPI 3.0.3, RFC 9457 errors | ADR-0010 |
| PLI-C-05 | Kratos v26.2.0 pinned; every Kratos behaviour relied upon is verified on that version | intent |
| PLI-C-06 | Secrets never in source or logs | org memory |

## Assumptions

| ID | Assumption | If wrong |
| --- | --- | --- |
| PLI-A-01 | The resolve endpoint is an acceptable rate-limited oracle (protects data at rest, not account existence) | Would require a proxy, contradicting PLI-C-01 → escalate |
| PLI-A-02 | Kratos v26.2.0 `http` courier exposes recipient, template type and template data (code) to the request body Jsonnet | Feasibility spike; blocker if false |
| PLI-A-03 | Kratos accepts `<base32>@x.invalid` for `format: email` | Feasibility spike; else choose a different reserved domain |
| PLI-A-04 | Admin identity update can set the new verifiable/recovery address as verified | Feasibility spike; else migration forces re-verification (degraded) |
| PLI-A-05 | One login identifier per customer | Multi-identifier is a later intent |

## Traceability (intent SC → requirements)

| SC | Requirements |
| --- | --- |
| 1 no contact in Kratos | FR-01, 03, 04, 14; NFR-01 |
| 2 encrypted vault | FR-02, 15, 16, 17; NFR-02, 03 |
| 3 passwords only to Kratos | FR-01; NFR-04; C-01 |
| 4 delivery via identity-service | FR-05, 06; NFR-10 |
| 5 mobile e2e both types | FR-07, 08, 09 |
| 6 admin search/view | FR-10, 11, 12; NFR-11 |
| 7 migration | FR-13, 14; NFR-12 |
| 8 limits, logs, fail closed | FR-04; NFR-05, 06, 07, 08 |

No orphan requirements: FR-18 (Could) and FR-19 (docs) trace to the intent's
out-of-scope note and to every SC respectively.
