# Constraint Register

| # | Constraint | Source |
| --- | --- | --- |
| C1 | Email must stay plaintext in Kratos (login lookup, verification/recovery mail) | Kratos design |
| C2 | v1 API changes are additive; deprecate before removing | docs/principles/05-api-guidelines.md |
| C3 | Secrets and PII never in source or logs | org memory |
| C4 | Erasure requests already recorded must be honoured by the migration | ADR-0011, Decree 13 Art. 16 / GDPR Art. 17 |
| C5 | Human review before merge | org memory |
