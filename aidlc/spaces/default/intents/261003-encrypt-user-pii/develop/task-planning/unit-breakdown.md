# Unit Breakdown — Encrypted personal information

| Unit | Scope | Requirements | Owner track | Depends on |
| --- | --- | --- | --- | --- |
| P1 | Compose infra: OpenBao + init/unseal job, policies, volumes, Make targets (`kek-rotate`, `keys-rewrap`), Keto OPL `reveal_customer_pii` | PII-FR-12, PII-NFR-05 | lead (deploy/) | — |
| P2 | Go crypto core: `envelope`, `localkms`, `openbao` adapter + token renewal, `KeyManager` port, DEK cache | PII-NFR-01/02/03/06/07 | Go | P1 (integration tests) |
| P3 | Go domain + persistence: `domain/pii` (validate, normalise, mask, redact), migration 0003, sqlc queries, repos | PII-FR-03, PII-NFR-04 | Go | — |
| P4 | Go use cases + HTTP: personal-info service, 6 handlers, route policies, audit events, `keys rewrap` CLI, config | PII-FR-01..07, 09 | Go | P2, P3 |
| P5 | Mobile personal-info screen | PII-FR-10 | Flutter | contract |
| P6 | Admin web masked card, reveal dialog, phone lookup | PII-FR-11 | React | contract |
| P7 | Smoke checks + docs (architecture, security, ADR-0011, API guide, data model) | PII-NFR-08/09 | lead | P1–P6 |

Walking skeleton order: P1 → P2/P3 → P4 → (P5 ∥ P6) → P7.
