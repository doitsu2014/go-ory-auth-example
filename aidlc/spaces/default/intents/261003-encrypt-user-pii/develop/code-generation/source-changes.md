# Source Changes — Encrypted personal information

| Area | Paths | Unit |
| --- | --- | --- |
| Infra | `deploy/openbao/{openbao.hcl,init.sh,policies/*.hcl}`, `deploy/compose/docker-compose.yml` (openbao, openbao-init, volumes, identity-service PII env + `APP_ENV: local`), `Makefile` (bao-init, bao-token, kek-rotate, keys-rewrap), `.gitignore` (`deploy/compose/.local/`), `deploy/ory/keto/namespaces.keto.ts` (`reveal_customer_pii`) | P1 |
| Contract | `api/openapi/identity-service.v1.yaml` (3 customer ops, 3 admin ops, 7 schemas, Permission enum) | — |
| Go | `services/identity-service/`: migration `0003_customer_pii.sql`, `db/queries/pii.sql`, `internal/domain/pii`, `internal/crypto/envelope`, `internal/app/{personalinfo,dekcache,ratelimit,keys}.go`, `internal/adapter/{openbao,localkms}`, `internal/adapter/postgres/pii.go`, `internal/adapter/httpapi/personalinfo.go`, CLI `keys rewrap` + `pii reapply-erasures`, regenerated `api.gen.go` + `sqlcgen`, edits to ports/errors/policy/problem/server/config/logger/main + tests | P2–P4 |
| Mobile | `apps/mobile/lib/features/profile/{domain/personal_info*.dart,presentation/personal_info_screen.dart,data/profile_repository.dart}`, routes, l10n (34 strings), tests | P5 |
| Admin web | `apps/admin-web/src/features/customers/{PersonalInfoCard,RevealDialog,PhoneLookup}.tsx`, `piiErrors.ts`, `auth/useAuthRedirect.ts`, `api/schema.d.ts`, queries, i18n, tests | P6 |
| Smoke + docs | `scripts/smoke.mjs` (+6 PII checks), `docs/architecture/08-pii-protection.md`, `docs/adr/0011-envelope-encryption-for-pii.md`, edits to 04-data, 05-deployment, 06-security (T16/T17, accepted risks), API guide, docs index | P7 |

## Verification (lead, 2026-10-03)

| Check | Result |
| --- | --- |
| Go build / vet / `test -race` / `test -race -tags integration -count=1` | pass (11 integration pkgs) |
| `make down && make up` → previously stored PII still decrypts | pass (OpenBao re-unsealed by `openbao-init`) |
| `node scripts/smoke.mjs` | 25/25 pass |
| Mobile analyze / test / integration | clean / 99 pass (+1 skipped tag) / 6/6 incl. personal-info |
| Web lint / typecheck / test / build | pass (77 tests) |
| OpenAPI Redocly lint | 0 errors (8 pre-existing warnings) |
