# Unit Breakdown

Scope for this implementation run (YOLO, recommended): roadmap Milestones 1–3
(walking skeleton, customer self-service, admin plane). Milestone 4
(hardening, load, dashboards) is left to Launch/Curate. Spikes S1–S3 are folded
into the units that need them and verified against the running stack.

| ID | Title | Req | Acceptance criteria | Files | Depends | Size |
| --- | --- | --- | --- | --- | --- | --- |
| U1 | Local infra stack | NFR-05, NFR-06, HC-02 | `make up` starts postgres (3 DBs, roles), kratos (+migrate, rendered config), keto (+migrate, OPL), mailpit; health green; admin ports bound to 127.0.0.1 only | `deploy/**`, `Makefile` | — | M |
| U2 | OpenAPI v1 contract | ADR-0010 | `api/openapi/identity-service.v1.yaml` covers every endpoint in docs/api; validates as OpenAPI 3.x | `api/openapi/*` | — | M |
| U3 | identity-service skeleton + authn | FR-08, FR-09, FR-10, NFR-03, NFR-07 | serve/migrate cmds; 3 ports (8080/8081/9090); plane-bound credential; whoami 403 aal2 mapping; `GET/PATCH /v1/me`; lazy upsert; webhook upsert; tests pass | `services/identity-service/**` | U1, U2 | L |
| U4 | Admin plane API | FR-06, FR-07, FR-11, FR-12 | Keto authorizer; AAL2 + 12 h cap; customers list/get/disable/enable/revoke; admins list/invite/role; audit; bootstrap CLI; login guard webhook (S3) | `services/identity-service/**` | U3 | L |
| U5 | Admin web | FR-05, FR-06, FR-13 | Login (browser flow) + AAL2 step-up + TOTP settings + recovery; guard via `/admin/v1/me`; customers/admins/audit pages; logout; no registration; build/lint/tests pass | `apps/admin-web/**` | U1, U2 (mocks), U4 (E2E) | L |
| U6 | Mobile app | FR-01–04, FR-10, FR-13 | Sign up, verify (code), sign in, recovery, profile view/edit, sign out via native flows; token in secure storage; analyze + tests pass | `apps/mobile/**` | U1, U2 (mocks), U3 (E2E) | L |
| U7 | End-to-end smoke | NFR-08 | Script registers a customer via Kratos API flow, calls `/v1/me`, bootstraps an admin, asserts admin-plane denials for a customer | `scripts/smoke.sh` | U3–U4 | S |

Critical path: U1 → U3 → U4 → U7. U5 and U6 run in parallel with U3/U4
against the contract and are integrated at U7.
