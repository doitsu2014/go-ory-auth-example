# Feasibility Assessment

## Technical feasibility

| Requirement(s) | Verdict | Notes / spike |
| --- | --- | --- |
| FR-01..04 mobile register/login/verify/recover | Feasible | Kratos native API flows; Dart client `ory_client` on pub.dev. Spike S1: render `ui.nodes` generically vs hand-built forms. |
| FR-05 admin login | Feasible | Browser flows from React SPA, `@ory/client` / `@ory/elements-react`. Cookie-domain setup is the main risk (R-01). |
| FR-06 admin role + AAL2 | Feasible | Role in Keto; AAL read from `session.authenticator_assurance_level` in the Go middleware. |
| FR-07 admin invitation | Feasible | Kratos admin API `createIdentity` (schema `admin`) + `createRecoveryLinkForIdentity`/code. |
| FR-08 session validation | Feasible | `FrontendApi.ToSession` with cookie or token; short TTL cache to meet NFR-01. |
| FR-09..12 profile, admin user mgmt, audit | Feasible | Plain Go + Postgres (pgx, sqlc). Disable = Kratos `state: inactive` + delete sessions. |
| NFR-05 one-command local stack | Feasible | Docker Compose: postgres, kratos(+migrate), keto(+migrate), mailpit, identity-service, admin-web. Mobile runs on emulator against host. |
| NFR-01 latency | Feasible with cache | Without cache every call adds a Kratos round trip (~2–10 ms locally). |

## Effort (ballpark, one developer familiar with Go)

| Slice | Effort |
| --- | --- |
| Local infra (compose, Kratos/Keto config, schemas) | 2–3 d |
| identity-service skeleton + auth middleware + profile | 4–5 d |
| Admin web (login, MFA, users list) | 4–5 d |
| Flutter app (register, login, verify, recover, profile) | 5–7 d |
| Tests, CI, docs upkeep | 3–4 d |
| **Total** | **~4 weeks** |

## Recommendation

**Proceed with spikes.** S1 (Flutter rendering of Kratos UI nodes) and S2
(SPA cookie/CORS setup on localhost) are each ≤ ½ day and should be the first
units in Develop.
