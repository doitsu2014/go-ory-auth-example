# Requirements — Feature diagrams

Source: [intent-statement](../intent-capture/intent-statement.md).

## Feature inventory (derived from code)

| ID | Feature | Actors | Surface |
| --- | --- | --- | --- |
| F01 | Customer registration and code delivery | Customer (mobile) | `POST /v1/auth/registration`, Kratos hooks pre/after-registration, courier |
| F02 | Customer verification | Customer (mobile) | Kratos verification flow |
| F03 | Customer login | Customer (mobile) | `POST /v1/auth/login`, after-login hook |
| F04 | Customer password recovery | Customer (mobile) | `POST /v1/auth/recovery`, `/v1/auth/recovery/code` |
| F05 | Customer settings and logout | Customer (mobile) | Kratos settings / logout |
| F06 | Request authentication (session/JWT) | All callers | middleware, Kratos whoami, Hydra JWKS |
| F07 | Customer profile | Customer | `GET/PATCH /v1/me` |
| F08 | Customer personal info (PII) | Customer | `GET/PUT/DELETE /v1/me/personal-info` |
| F09 | Admin sign-in, recovery, settings, logout | Admin (admin-web) | Kratos browser flows, `GET /admin/v1/me` |
| F10 | Admin management | Admin (owner) | `/admin/v1/admins`, `/admins/{id}/role` |
| F11 | Customer management | Admin | `/admin/v1/customers`, lookup, detail, disable/enable, sessions |
| F12 | Admin PII access | Admin | `/admin/v1/customers/{id}/personal-info`, `/reveal` |
| F13 | Audit log | Admin, system | `GET /admin/v1/audit-events`, audit writer |
| F14 | Service clients | Admin | `/admin/v1/service-clients…` (Hydra) |
| F15 | Machine-to-machine API | Partner service | `/m2m/v1/customers/{id}`, `/m2m/v1/audit-events` |
| F16 | Background jobs | System | key rotation, login/name migrations |

## Requirements

| ID | Statement | Type | Priority | Acceptance criteria | Verification |
| --- | --- | --- | --- | --- | --- |
| R1 | `docs/features/README.md` has a system use-case diagram, a context diagram, and a catalogue table linking F01–F16 | Functional | Must | The file exists. Both diagrams parse. Every feature row links a page that exists. | Link check + Mermaid parse |
| R2 | Each feature page has a functional flowchart | Functional | Must | ≥1 `flowchart` block per page | grep + parse |
| R3 | Each feature page has ≥1 sequence diagram | Functional | Must | ≥1 `sequenceDiagram` block per page | grep + parse |
| R4 | Each diagram matches the current code | Functional | Must | Every page has a "Code references" section, and every cited path exists | Script: cited paths exist |
| R5 | Error branches are shown | Functional | Should | The main failure paths appear as `alt`/`opt` blocks or flowchart branches | Review |
| R6 | Security-relevant facts are visible | Functional | Should | Keto checks, audit events, and points where PII is encrypted or decrypted are labelled | Review |
| R7 | Mermaid is valid and renders on GitHub | Non-functional | Must | All blocks parse with the Mermaid parser | mermaid-cli |
| R8 | Discoverable | Functional | Must | `docs/README.md` links the catalogue, and stale architecture diagrams link to the new page | Link check |
| C1 | No code, config or OpenAPI changes | Constraint | Must | The diff touches only `docs/` and `aidlc/` | `git diff --stat` |
| A1 | English, consistent with the existing docs | Assumption | — | — | — |

All requirements trace to the success criteria in the intent statement, and
none are orphans.
