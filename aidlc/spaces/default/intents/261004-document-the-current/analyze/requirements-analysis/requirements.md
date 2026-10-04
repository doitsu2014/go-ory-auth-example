# Requirements — Document the pseudonymous login model

| ID | Pri | Requirement | AC |
| --- | --- | --- | --- |
| DOC-01 | M | Sequence diagram: customer login | Lanes Client, Identity API, OpenBao, PII Vault DB, Ory Kratos. Shows resolve (normalise, HMAC on `identity-login-pseudonym`, no DB write), the Kratos native login with the pseudonym + password directly from the client, and the session token |
| DOC-02 | M | Sequence diagram: registration + code delivery | Resolve with `purpose: registration` (seal + insert), pre-registration webhook (vault check, 4049001/4049002), after-registration bind, http courier → decrypt → email/SMS, verification |
| DOC-03 | M | Sequence diagram: `/me` | whoami session (cached), vault read by pseudonym, lazy bind, batch decrypt, response `login {type, value}` |
| DOC-04 | M | Sequence diagram: forgot password | Resolve (`purpose: recovery`), Kratos recovery flow with the pseudonym, courier → decrypt → send, code submit, privileged settings flow. Kratos stays silent for unknown recipients |
| DOC-05 | S | Sequence diagram: admin lookup + reveal | Body lookup → HMAC → Kratos `credentials_identifier`; masked batch decrypt; reveal with audit commit before the response |
| DOC-06 | M | Per-hop API table | Client→Identity API, Client→Kratos, Identity API→OpenBao, Identity API→Vault DB, Identity API→Kratos admin, Kratos→Identity API (webhooks, courier), Identity API→Email/SMS |
| DOC-07 | M | Pros / cons | Each item concrete and linked to a control or risk |
| DOC-08 | M | Comparison with proxy + UUID model | Table across password path, identifier, DB lookup per login, Kratos features, enumeration, availability, and complexity |
| DOC-09 | M | Future directions | Prioritised and actionable; references existing follow-ups (SEC-C07/C09/C12, attestation, passkeys, …) |
| DOC-10 | M | Linked from 01, 03, 08, ADR-0013, docs index | Links resolve |
