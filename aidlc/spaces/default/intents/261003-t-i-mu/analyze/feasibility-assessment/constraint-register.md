# Constraint Register

## Hard constraints

| ID | Constraint | Source |
| --- | --- | --- |
| HC-01 | Go backend, PostgreSQL, Ory stack, React admin, Flutter mobile | Intent |
| HC-02 | Kratos/Keto admin APIs never internet-reachable | NFR-04, org least-privilege |
| HC-03 | Secrets via env/secret store only; never in git or logs | Org |
| HC-04 | Browser clients use browser flows; native clients use API flows | Ory docs (research RQ2/RQ3) |
| HC-05 | Each component owns its database; no cross-DB reads | NFR-06 |
| HC-06 | Ory licence Apache-2.0 (self-hosted); pin image versions | Licensing |

## Risks

| ID | Risk | L | I | Mitigation |
| --- | --- | --- | --- | --- |
| R-01 | Cookie domain/CORS misconfig blocks admin login | H | M | Same-site layout (`*.example.local` / localhost), spike S2, documented config |
| R-02 | Session check per request overloads Kratos | M | M | In-process cache (TTL ≤ 30 s, keyed by token hash); revocation tolerance documented |
| R-03 | Registration webhook fails → missing profile | M | L | Idempotent lazy upsert on first request |
| R-04 | Kratos upgrade breaks config/migrations | M | M | Pin versions; run `kratos migrate sql` as a separate job; upgrade notes in runbook |
| R-05 | Admin account takeover | L | H | AAL2 mandatory, non-selectable admin schema, audit log, short admin session lifespan |
| R-06 | Mobile token theft from device | L | H | `flutter_secure_storage` (Keychain/Keystore), no logging of tokens, server-side revocation |
| R-07 | Email deliverability in prod | M | M | Courier via SMTP provider; Mailpit locally |
