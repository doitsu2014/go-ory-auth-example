# Intent Statement — Customer name out of Kratos, into encrypted PII

## Problem

Customer phone, date of birth, address and national id are envelope-encrypted
in `customer_pii`. The customer's **real name** is not: it sits in plaintext in
Kratos `identities.traits.name` (the `customer` schema declares it, mobile
sign-up and the seed send it), and the seed also copied it into the plaintext
`profile.display_name`. Kratos does not need a name to work (only the email,
for login lookup and mail), so the name is avoidable plaintext PII in two places.

## Users

| User | Needs |
| --- | --- |
| Customer | Enter / change / erase their name with the same protection as their phone number |
| Admin (admins, super_admins) | See the name masked; reveal it with a reason (audited), like other PII |
| Support | Sees masked name only |
| Operator | Move names of existing customers out of Kratos without losing data |

## Success criteria

1. No customer name in Kratos: the `customer` schema has no `name` trait and
   registration with `traits.name` is rejected.
2. The name is stored as an encrypted field of personal info (same DEK,
   AES-256-GCM, AAD binding, mask / reveal / erase / crypto-shred as phone).
3. A one-shot, idempotent migration copies existing Kratos names into
   encrypted PII and then strips them from Kratos; `./dev up` runs it.
4. `display_name` stays an optional nickname; seeds no longer put real names
   there or in Kratos.
5. Docs state where plaintext PII still lives (email in Kratos) and the
   controls and accepted risk for it.

## Out of scope

Admin names (employee data in the `admin` schema), search by name, encrypting
email (Kratos needs it in plaintext), production schema-rollout automation.
