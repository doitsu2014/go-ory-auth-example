# Source Changes — M2M with Hydra

| Area | Paths | Unit |
| --- | --- | --- |
| Infra | `deploy/ory/hydra/hydra.yml`, `deploy/postgres/provision/hydra.sh`, compose (`hydra-db`, `hydra-migrate`, `hydra`, networks `hydra` + `hydra-db`, identity-service M2M env incl. `M2M_CLIENT_TAG_KEY`), `.env.example`, Makefile (`infra-up`, `health`), Keto OPL `manage_service_clients` | H1 |
| Contract | `api/openapi/identity-service.v1.yaml`: `/m2m/v1/customers/{id}`, `/m2m/v1/audit-events`, `/admin/v1/service-clients*`, `machineToken` scheme, `MachineCustomer`, `MachineAuditEvent(Page)`, `ServiceClient*`, Permission `manage_service_clients`, problem codes `invalid_token`/`insufficient_scope`/`invalid_request` | — |
| Go | `domain/machine`, `domain/audit/machine.go`, `app/{serviceclients,machine}.go`, `adapter/hydra/*` (admin, JWKS, verifier, status cache, integrity tag), `adapter/httpapi/{machine,server_machine}.go` + policy/middleware/problem changes, `audit.sql` allowlist, config/logger, CLI `clients create`, module `github.com/go-jose/go-jose/v4` | H2, H3 |
| Admin web | `src/features/service-clients/*`, routes, nav, `ConfirmDialog` props, `shared/env.ts` (`VITE_HYDRA_PUBLIC_URL`), i18n, tests | H4 |
| Smoke + docs | `scripts/smoke.mjs` (+8 M2M checks), `docs/architecture/09-machine-access.md`, `docs/adr/0012-*`, ADR-0002 note, 05-deployment, 06-security (T18/T19, accepted risks), API guide, docs index + glossary | H5 |
