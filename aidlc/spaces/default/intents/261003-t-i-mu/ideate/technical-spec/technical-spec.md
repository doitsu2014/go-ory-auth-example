# Technical Specification

Component details: `docs/architecture/02-components.md` and
`docs/principles/02..04-*.md`. Cross-cutting: `docs/architecture/06-security.md`,
`07-cross-cutting.md`. Work breakdown seeds: `docs/roadmap.md`.

## 1. Traceability

| Req | Component(s) | Interface(s) | Test approach |
| --- | --- | --- | --- |
| FR-01 | Mobile, Kratos | `/self-service/registration/api` | Mobile integration (emulator + compose) |
| FR-02 | Mobile, Kratos | `/self-service/login/api` | Mobile integration |
| FR-03 | Mobile, Kratos, SMTP | `/self-service/verification` | Integration (Mailpit API) |
| FR-04 | Mobile, Kratos | `/self-service/recovery`, settings | Integration |
| FR-05 | Admin web, Kratos | `/self-service/login/browser` | Playwright |
| FR-06 | identity-service, Keto | `/admin/v1/*` middleware | HTTP authz matrix tests |
| FR-07 | identity-service, Kratos admin, Keto | `POST /admin/v1/admins`, CLI | Integration (testcontainers) |
| FR-08 | identity-service | `Authenticate` middleware | Unit + integration |
| FR-09 | identity-service, Postgres | webhook, `EnsureProfile` | Integration (duplicate delivery) |
| FR-10 | identity-service | `GET/PATCH /v1/me` | Integration |
| FR-11 | identity-service, Kratos admin | customers endpoints | Integration |
| FR-12 | identity-service, Postgres | `audit_event` | Integration |
| FR-13 | Clients, Kratos | logout endpoints | E2E |
| NFR-01 | identity-service | session cache | k6 |
| NFR-02/04 | all | config, network policy | Security checklist review |
| NFR-03 | repo, logs | gitleaks, redaction | CI + unit test on redaction handler |
| NFR-05/06 | deploy | compose | CI smoke (`make up && make smoke`) |
| NFR-07 | identity-service | OTel | Manual trace check |
| NFR-08 | all | — | CI report |

All Must requirements appear.

## 2. Database record (db-postgres skill)

| Item | Choice |
| --- | --- |
| Engine | PostgreSQL 16+ (one cluster; DBs `kratos`, `keto`, `identity`) |
| Driver | `jackc/pgx/v5` |
| Query layer | `sqlc` (pgx/v5 target) — no ORM |
| Migrations | `pressly/goose/v3`, SQL files embedded; forward-only; expand/contract for destructive changes |
| Pooling | `pgxpool` per process, `max_conns ≈ 2 × CPU` (start 8), `max_conn_idle_time 5m`; no PgBouncer in v1 (if added: transaction mode with `QueryExecModeCacheDescribe`/no server-side prepares) |
| Timeouts | `statement_timeout 5s`, `idle_in_transaction_session_timeout 10s`, `lock_timeout 3s` for migrations |
| Roles | `identity_migrator` (DDL), `identity_app` (DML; audit append-only) |
| Connection | single DSN from env, `sslmode=require` outside local, `application_name=identity-service` |
| Tests | Testcontainers Postgres, migrations in setup, truncate between tests |
| Ops | `pg_stat_statements` on; PITR backups; monthly restore test |

## 3. Cross-cutting summary

Security: ASVS L2, AAL2 admins, private admin APIs, redaction (06-security).
Observability: slog JSON, OTel, Prometheus, health/readiness (07-cross-cutting §7.1).
Performance budget: p95 < 150 ms; Ory call timeout 2 s; DB statement 5 s.
Data lifecycle: 04-data §4.5.

## 4. Work breakdown seeds

Spikes S1–S3, then U1–U19 in `docs/roadmap.md` (walking skeleton first).
