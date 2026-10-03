# Source Changes

Branch `feat/walking-skeleton` (not committed; owner reviews and commits).

| Unit | Paths | Summary |
| --- | --- | --- |
| U1 | `deploy/compose/docker-compose.yml`, `deploy/compose/.env.example`, `deploy/postgres/init/01-databases.sh`, `deploy/ory/kratos/{kratos.yml.tmpl,identity-schemas/*,webhooks/*}`, `deploy/ory/keto/{keto.yml,namespaces.keto.ts}`, `Makefile` | Local stack: Postgres 16 (3 DBs/roles), Kratos v26.2.0 (+migrate, rendered config), Keto v26.2.0 (+migrate, OPL), Mailpit; admin ports on 127.0.0.1 |
| U2 | `api/openapi/identity-service.v1.yaml` | OpenAPI 3.0.3 contract; Redocly lint clean |
| U3+U4 | `services/identity-service/**` (63 files) | Go service: cobra CLI (serve/migrate/admin bootstrap/healthcheck), hexagonal layout, hand-written Kratos/Keto adapters, session cache, plane-bound auth, AAL2 + 12 h cap + MFA-enrolment rule, Keto authz, customers/admins/audit, webhooks incl. interrupting login guard, SMTP mailer, goose migrations, sqlc, oapi-codegen strict server, distroless Dockerfile |
| U5 | `apps/admin-web/**` (92 files) | React 19 + Vite 7 + TS strict; custom Kratos node renderer; browser flows; login/AAL2/settings/recovery/verification/error; customers/admins/audit; i18n vi/en; Tailwind v4 |
| U6 | `apps/mobile/**` (114 files) | Flutter app: native flows via `ory_client` 1.22; sign up/verify/sign in/recovery/profile/settings/sign out; secure storage; Riverpod + go_router; i18n vi/en |
| U7 | `scripts/smoke.mjs` | 19-check end-to-end smoke against the live stack |
| Docs | `docs/**`, `README.md` | Deviation sync (adapters, OpenAPI 3.0.3, config template, repo structure) |
