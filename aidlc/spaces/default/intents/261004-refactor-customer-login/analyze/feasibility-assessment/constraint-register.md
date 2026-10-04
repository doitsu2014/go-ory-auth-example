# Constraint register

| ID | Constraint | Source |
| --- | --- | --- |
| K-01 | Kratos `traits.login_id` must keep the pattern `^[a-z2-7]{52}@login\.invalid$` (customer.v2 schema) | deploy/ory/kratos/identity-schemas |
| K-02 | The courier webhook resolves recipients by Kratos handle, so `pseudonym` stays the vault primary key | ADR-0013 courier |
| K-03 | The pre-registration webhook stays: it blocks direct Kratos registrations with arbitrary handles | SEC-C07 |
| K-04 | Existing handles equal their HMAC (pre-0007). They cannot change without a two-patch Kratos rewrite (follow-up) | data |
| K-05 | Never echo a Kratos flow or node value to an unauthenticated caller | spike |
| K-06 | Admin web and admin identities are unchanged | scope |
