# 7. Cross-cutting Concerns

## 7.1 Observability

| Signal | Standard | Details |
| --- | --- | --- |
| Logs | JSON via Go `log/slog` | Fields: `ts`, `level`, `msg`, `request_id`, `trace_id`, `identity_id` (never email/token), `route`, `status`, `duration_ms` |
| Traces | OpenTelemetry (OTLP) | HTTP server spans, outbound spans to Kratos/Keto, pgx tracer; W3C `traceparent` propagated from clients when present |
| Metrics | Prometheus (`/metrics`, private) | RED per route, Ory call latency/error, session cache hit ratio, DB pool stats; Kratos & Keto expose their own `/metrics/prometheus` |
| Health | `/healthz` (process up), `/readyz` (DB + Kratos + Keto reachable) | Used by orchestrator probes |

Alerts (actionable only): API 5xx rate > 2 % for 5 min; p95 > 300 ms for
10 min; Kratos unreachable; webhook failures > 10/min; DB pool saturation.

## 7.2 Configuration

- 12-factor: all config from env vars, parsed once into a typed struct at
  startup (`caarlos0/env`), validated, and the process exits on invalid config.
- Same binary/image for every environment; only env differs.
- Clients: admin web via `VITE_*` build vars (`VITE_KRATOS_PUBLIC_URL`,
  `VITE_API_URL`); mobile via `--dart-define-from-file=env/<flavour>.json`.

## 7.3 Error model

- identity-service returns **RFC 9457 `application/problem+json`** with a stable
  machine `code` (e.g. `aal2_required`, `not_admin`, `email_not_verified`).
- Clients map `code` → localised message; never show raw server text.
- Kratos flow errors are rendered from Kratos `ui.messages[].id`.

## 7.4 Internationalisation

- UI languages: Vietnamese (default) and English for both clients.
- Kratos emails (courier templates) provided in both languages; template
  chosen from the identity's `locale` trait/profile (later iteration).

## 7.5 Testing strategy

| Level | Scope | Tooling | Runs in |
| --- | --- | --- | --- |
| Unit | Domain & use cases with fakes for ports | Go `testing` + `testify`; Vitest + React Testing Library; `flutter_test` | every commit |
| Integration | identity-service against real Postgres, Kratos, Keto | `testcontainers-go` (or compose in CI) | every PR |
| Contract | Server conforms to OpenAPI; clients generated from it | `oapi-codegen` strict server, schema diff check (`oasdiff`) | every PR |
| E2E web | Admin login + MFA + disable customer | Playwright against compose stack, Mailpit API for codes | every PR (main) |
| E2E mobile | Register → verify → login → profile | `integration_test` / Patrol on emulator | nightly |
| Security | SAST & deps | `gosec`, `govulncheck`, `gitleaks`, `pnpm audit` | every PR |
| Load | NFR-01 | k6 script | before release |

Every requirement id (FR-xx/NFR-xx) maps to at least one test; test names
include the id (e.g. `TestFR06_AdminRequiresAAL2`). Happy-path floor per component.

## 7.6 CI

Same commands locally and in CI (via `Makefile`/`Taskfile`):
`make lint`, `make test`, `make test-integration`, `make e2e`.
Path filters run only the affected app's jobs; docs-only changes run markdown
lint + link check.
