# Architecture Document

The architecture is published for the team in `docs/` (the owner asked for a
docs folder). This record is the stage summary and index; the linked files are
the normative content.

## Options considered

| Option | Verdict | Reason |
| --- | --- | --- |
| A. Kratos sessions + Keto + Go API | **Chosen (recommended, YOLO)** | Simplest correct first-party model; matches Ory guidance |
| B. Kratos + Hydra (OAuth2 everywhere) | Rejected v1 | System-browser on mobile, consent + refresh lifecycle; revisit for third parties |
| C. Kratos + Oathkeeper | Deferred | Worth it with ≥ 2 backend services |
| D. Go BFF proxies auth | Rejected | Passwords transit our code; reimplements Kratos flows |

## Normative content

| Topic | File |
| --- | --- |
| Context, containers (Mermaid), options, quality attributes | `docs/architecture/01-overview.md` |
| Component responsibilities, Keto model, responsibility matrix | `docs/architecture/02-components.md` |
| Data flow: sequence diagrams for every auth path, authz pipeline | `docs/architecture/03-auth-flows.md` |
| Data ownership, schemas, DDL | `docs/architecture/04-data.md` |
| Deployment diagrams (local + prod), network policy | `docs/architecture/05-deployment.md` |
| Threat model, controls | `docs/architecture/06-security.md` |
| Observability, config, errors, testing | `docs/architecture/07-cross-cutting.md` |

## Failure modes

| Failure | Behaviour |
| --- | --- |
| Kratos down | Logins fail; API returns 503 (fail closed) except cached sessions ≤ 30 s |
| Keto down | Admin endpoints 503; customer endpoints unaffected |
| identity-service down | Registration still succeeds (webhook ignored); profile created lazily later |
| Postgres down | Everything fails; `/readyz` false; PITR restore runbook |
| SMTP down | Verification/recovery delayed; Kratos courier retries |

## Requirement coverage

FR-01..04 → Kratos API flows + mobile; FR-05/06 → browser flows + AAL2 + Keto;
FR-07 → admin API + bootstrap CLI; FR-08 → middleware; FR-09..12 → identity
DB; FR-13 → Kratos logout; NFR-01 → cache; NFR-02..04 → security doc; NFR-05 →
compose; NFR-06 → DB per component; NFR-07 → OTel; NFR-08 → test strategy.

## Security review

See `security-review.md` in this folder.
