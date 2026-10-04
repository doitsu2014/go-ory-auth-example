# Implementation plan

Order: U1 → U2 → U3 → U4 (Go build green, `make test` green) → U6 (rebuild
the stack, run integration tests and smoke) → U5 (`flutter analyze`, `flutter
test`) → U7.

Verification commands:

- `cd services/identity-service && go build ./... && go vet ./... && go test -race ./...`
- `go test -race -count=1 -tags integration ./...` (local stack)
- `make up && node scripts/smoke.mjs`
- `cd apps/mobile && flutter analyze && flutter test`
- `cd apps/admin-web && npm run generate:api && npm run typecheck` (if present)

Rollout: identity-service and the app ship together. The old app build
calls the removed endpoint and gets 404, so a forced app update is needed.
This is acceptable for the example project; production would keep the old
endpoint behind a flag for one release.
