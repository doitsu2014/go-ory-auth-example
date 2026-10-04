# Intent Capture — Questions

Mode: **yolo**. No questions were put to the human; each answer below is the
recommended option, recorded as a `Recommendation`. The human's own input is
the request: *"đi theo hướng B và làm nó thật chuẩn chỉ"* (go with option B and
do it properly), following the option table given in the previous turn:

> **B. Pseudonymize identifiers**: Kratos stores only `HMAC(normalized email)`
> (e.g. `<hex>@pseudo.invalid`); identifier rewriting before Kratos; courier
> `delivery_strategy: http` → identity-service webhook that resolves the real
> address (encrypted) and sends mail/SMS.

## Q1. What problem does this solve, and for whom?

- A. Remove customer contact data (email, phone) from the Kratos database so a
  Kratos DB dump, backup or over-privileged SQL session exposes no customer
  contact identifier; the real values live only encrypted in identity-service.
- B. Encrypt the whole Kratos database (TDE).
- C. Hide email only from admins.
- X. Other

[Answer]: A — Recommendation. B was option A in the prior analysis (storage-level),
explicitly not chosen. C does not address data at rest.

## Q2. Which identifiers are covered?

- A. Customer email **and** customer phone number as login identifiers (the
  user's example "user đăng nhập = sđt"), through one uniform pseudonym scheme.
- B. Customer email only.
- C. Customer and admin emails.
- X. Other

[Answer]: A — Recommendation. One scheme serves both kinds, so adding phone
costs an SMS delivery adapter and UI, not a second design. Admins stay out
(employee data, browser flows, invitation mail; see Q4).

## Q3. How is the real identifier turned into the pseudonym without passwords passing through our code?

- A. Clients ask identity-service for the pseudonym of a typed address
  (`POST /v1/auth/identifiers`), then submit **the pseudonym** to Kratos public
  directly, together with the password / code. Passwords keep going only to
  Kratos.
- B. A reverse proxy / BFF in front of Kratos rewrites the identifier inside
  every self-service request body (passwords transit our code).
- C. Clients compute the HMAC themselves (key on the device).
- X. Other

[Answer]: A — Recommendation. B contradicts a standing rule
(02-components §2.3 "Never receive or store passwords; proxy Kratos
self-service flows", 01-overview §1.5 option D rejected). C embeds a secret
in the app, forbidden by §2.5. The "proxy/BFF" wording in option B is
therefore realised as a *pseudonym resolution endpoint*, not a body-rewriting
proxy. This is the one deliberate deviation from the earlier sketch and is
recorded as such.

## Q4. What is explicitly out of scope?

- A. Admin identities (keep `admin` schema with plaintext work email);
  passwordless login (code as first factor); production SMS provider
  contract beyond an adapter port; encrypting Kratos sessions' IP/user agent.
- B. Nothing out of scope.
- X. Other

[Answer]: A — Recommendation.

## Q5. Hard deadlines, budget or regulatory constraints?

- A. No date. Regulatory driver: Vietnam PDPL 91/2025/QH15 (effective
  2026-01-01) and Decree 13/2023 security measures; compliance may require
  field-level protection of customer contact data in every store. Counsel
  confirms article numbers.
- X. Other

[Answer]: A — Recommendation.

## Q6. What happens if we do nothing?

- A. The accepted risk in 06-security §6.7 ("Customer email stays plaintext in
  the Kratos DB") remains; any Kratos DB/backup leak discloses every
  customer's email, and phone login (when added) would add phone numbers to
  the same exposure. The risk's own revisit trigger is "login by a
  pseudonymous identifier is acceptable" — this intent is that trigger.

[Answer]: A — Recommendation.

## Q7. Existing customers?

- A. Ship a migration CLI that moves each existing customer's plaintext email
  into the encrypted vault and replaces Kratos traits/identifiers with the
  pseudonym, idempotent and crash-safe (same discipline as
  `pii migrate-kratos-names`).
- B. Only new registrations.
- X. Other

[Answer]: A — Recommendation. "Làm chuẩn chỉ" implies no plaintext left behind.

## Ambiguity / contradiction check

- "proxy/BFF" (sketch) vs "passwords never pass through identity-service"
  (standing rule) → resolved by Q3-A; no contradiction carried forward.
- 01-overview §1.1 lists "SMS/phone login" as a v1 non-goal → this intent
  lifts it for customers; the overview is updated in Curate/Develop docs.
- 08-pii §8.1 "phone numbers are self-declared, unverified, not unique" refers
  to the *personal-info* phone field, not the login phone. The two stay
  separate fields; a login phone is verified and unique (Kratos identifier).
