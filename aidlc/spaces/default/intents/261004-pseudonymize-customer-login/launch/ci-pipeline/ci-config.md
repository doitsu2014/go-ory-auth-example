# CI Pipeline — Pseudonymous customer login identifiers

The repo had no CI. New file: `.github/workflows/ci.yml` (GitHub Actions),
built only from the commands developers already run.

## Build contract (discovered)

| Area | Commands |
| --- | --- |
| identity-service | `make -C services/identity-service generate` (oapi-codegen + sqlc via `go tool`), `gofmt -l`, `make vet` (incl. `-tags integration`), `make test` (`-race`), `make build`, `make test-integration` (stack) |
| admin-web | `pnpm install --frozen-lockfile`, `pnpm generate:api`, `typecheck`, `lint`, `test --run`, `build` |
| mobile | `flutter pub get --enforce-lockfile`, `dart format --set-exit-if-changed`, `flutter analyze`, `flutter test` |
| stack | `make up`, `make bao-token`, `make login-migrate`, `make -C services/identity-service test-integration`, `node scripts/smoke.mjs`, `make reset` |

## Pipeline (fail fast, cheapest first)

| Job | Gate | Needs |
| --- | --- | --- |
| `generated` | Regenerate OpenAPI server, sqlc and `schema.d.ts`; `git diff --exit-code` | — |
| `go` | gofmt, vet (+integration tag), unit tests with `-race`, build | — |
| `admin-web` | typecheck, lint, tests, build | — |
| `mobile` | format check, analyze, tests | — |
| `stack` | Compose stack from the commit, OpenBao token, login migration, Go integration and e2e, smoke (47 checks incl. the PLI-NFR-01 SQL probe); logs on failure; always `make reset` | all four above |

## Determinism

- **Go:** `go-version-file` (go.mod `go 1.27.1`); tools pinned through
  `go tool` in go.mod.
- **Node 24:** pnpm comes from `packageManager` (`pnpm@10.33.0`), with a
  frozen lockfile.
- **Flutter:** pinned to `3.49.0-1.0.pre-218` on the `main` channel (the
  team's version), with an enforced lockfile.
- **Images:** pinned in compose (`oryd/kratos:v26.2.0`, `openbao/openbao:2.4.1`,
  `postgres:16-alpine`, `axllent/mailpit:v1.27`, …).
- **Actions:** pinned by major tag. Hardening follow-up: pin to commit SHAs
  (Dependabot can keep them current).
- **Tests:** unit tests use fixed clocks; e2e uses unique addresses per run.
  The flaky verification race found during Launch (L4) is fixed.

## Secrets and permissions

- `permissions: contents: read` only. The workflow needs no repository
  secrets.
- The stack job uses `deploy/compose/.env.example`: local-only values,
  documented as such. They are created by `make env` and never echoed.
- Courier and webhook keys and the OpenBao token exist only inside the
  ephemeral runner. OpenBao runs in local mode (unseal key on a volume that
  is deleted by `make reset`).

## Green run

GitHub Actions cannot run from this environment. Every job's commands were
run locally on this branch:

- `generated`: regeneration produced byte-identical files (no drift).
- `go`: gofmt clean (after the formatting fix it caught), `make vet`,
  `make test` and `make build` green.
- `admin-web`: typecheck, lint, test (130) and build green.
- `mobile`: format clean, analyze clean, test 174 passed / 1 skipped.
- `stack`: `./dev up` (equivalent to `make up` + migrations + seed),
  migration `failed=0`, `make test-integration` green twice, smoke 47/47.

The first run on GitHub must be checked once the branch is pushed. Pushing
is the owner's step.
