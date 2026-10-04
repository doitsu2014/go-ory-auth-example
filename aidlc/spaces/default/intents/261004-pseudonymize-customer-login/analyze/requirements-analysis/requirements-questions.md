# Requirements Analysis — Questions

Mode: **yolo**. Each answer is the recommended option, recorded as a
`Recommendation`; nothing here was asked of the human.

## Q1. Shape of the pseudonym stored in Kratos?

- A. `<base32(HMAC-SHA256)>@<reserved domain under .invalid>`: Kratos-valid
  `format: email`, undeliverable by any real MTA (RFC 6761 `.invalid`), same
  shape for email and phone so Kratos uses one `email` channel for both.
- B. Raw hex string, no `@` (needs a non-email trait → no `via: email`
  verification/recovery for it).
- C. Keep `format: tel` for phones (impossible: a pseudonym is not a number).

[Answer]: A — Recommendation.

## Q2. Rename the trait?

- A. Yes: `traits.email` → `traits.login_id`; schema id stays `customer`
  (served from a new `customer.v2.json`). Phone users make `email` a lie.
- B. Keep `traits.email`.

[Answer]: A — Recommendation. Migration has to rewrite the trait anyway.

## Q3. When is the real address persisted?

- A. Only when the client resolves with `purpose: registration`; other
  purposes compute the pseudonym without writing. Unbound entries are purged
  after 24 h if Kratos has no identity with that pseudonym.
- B. On every resolution.

[Answer]: A — Recommendation (data minimisation; no storage of mistyped
sign-in addresses).

## Q4. Customer-visible contact on `/v1/me`?

- A. `login: {type, value}` decrypted for the owner; `email` kept but
  optional (set only when `type = email`), marked deprecated.
- B. Remove `email`.

[Answer]: A — Recommendation (first-party clients upgrade together, but an
optional field keeps older builds rendering).

## Q5. Admin search by contact?

- A. Body-only lookup (`POST /admin/v1/customers/lookup` gains `login` input);
  `?email=` on `GET /admin/v1/customers` is removed — PII never in URLs.
- B. Keep `?email=` and translate.

[Answer]: A — Recommendation (aligns with 08-pii §8.3 "never the URL").

## Q6. Registration with an identifier that is not a vault-bound pseudonym?

- A. Rejected by a pre-persist registration webhook (validation error on the
  identifier node), if Kratos v26.2.0 supports it; else accepted but
  undeliverable and reported by a metric — decided in feasibility.

[Answer]: A — Recommendation.

## Q7. Admin identities and the shared courier?

- A. Kratos's courier is global: switching it to `http` routes admin
  verification/recovery mail through identity-service too. The dispatcher
  sends to plaintext admin addresses unchanged; only pseudonym recipients are
  resolved.

[Answer]: A — Recommendation.

## Q8. Phone delivery?

- A. `SMSSender` port; local adapter writes the message to Mailpit as an email
  to `<e164>@sms.local` (visible in the Mailpit UI, testable); production
  adapter is a configurable HTTP provider stub behind the port.

[Answer]: A — Recommendation.

## Q9. Login phone vs personal-info phone

- A. Separate. Login phone is verified + unique (Kratos identifier);
  `personal_info.phone_number` stays self-declared. No implicit sync.

[Answer]: A — Recommendation.

## Q10. Change of login identifier

- A. Not built in the app (Could). Server side: a settings-flow trait change
  to an unbound/unknown pseudonym must not break anything (delivery fails
  closed, address stays unverified).

[Answer]: A — Recommendation.

## Contradiction check

- Q4-A changes `Me.email` from required to optional → API contract change,
  recorded in the API stage; no contradiction with the intent.
- Q7 makes identity-service a mail dependency for Kratos flows; availability
  NFR added (Kratos retries courier messages).
