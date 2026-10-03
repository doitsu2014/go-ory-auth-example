# Implementation Plan

## Approach

1. U1 + U2 first (shared foundations, written by the lead).
2. Three parallel tracks against the contract: Go service (U3→U4), admin web
   (U5), mobile (U6). Each track verifies with its own commands and against
   the running compose stack.
3. U7 smoke ties them together; findings fed back to tracks.

## Versions (checked 2026-10-03)

| Item | Version |
| --- | --- |
| Ory Kratos / Keto images | `oryd/kratos:v26.2.0`, `oryd/keto:v26.2.0` |
| PostgreSQL | `postgres:16-alpine` |
| Go | 1.27 (local toolchain) |
| Node | 24, pnpm |
| Flutter | local toolchain (main channel 3.49 pre) |
| `@ory/client-fetch` / `@ory/elements-react` | 1.22.x / 1.2.x |
| Dart `ory_client` | 1.22.x |

## Deviations from the design (recorded up front)

- **Go Ory integration via thin hand-written HTTP adapters** instead of
  `kratos-client-go` (last release Feb 2025, lags Kratos v26) or
  `keto-client-go` (unmaintained). The service needs ~10 Ory endpoints; behind
  ports this is less code and avoids SDK/server skew. ADR-0009 text is updated.
- Kratos config secrets: the webhook key is rendered into the config by an init
  container (`sed` placeholder), following 05-deployment §5.3.

## Conventions

As in `docs/principles/*`. Generated code committed. Tests name requirement ids.

## Commands

| Unit | Build | Lint | Test |
| --- | --- | --- | --- |
| U1 | `make up` | `docker compose config -q` | `make health` |
| U3/U4 | `go build ./...` | `go vet ./...` (+ golangci-lint if installed) | `go test -race ./...` |
| U5 | `pnpm build` | `pnpm lint` | `pnpm test` |
| U6 | `flutter build` (debug) | `flutter analyze` | `flutter test` |
| U7 | — | — | `scripts/smoke.sh` |

## Review strategy

Per-track self-verification, then a code-review stage over the full diff
(correctness + security). Nothing is committed or merged without the owner.
