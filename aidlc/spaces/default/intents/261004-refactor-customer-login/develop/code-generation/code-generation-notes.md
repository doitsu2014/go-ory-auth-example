# Code generation notes

## Deviations from the spec

- **sqlc column order:** `lookup_key` is selected last, matching the
  table's column order, so sqlc keeps returning the shared `LoginIdentifier`
  model.
- **Session passthrough:** `x-go-type: json.RawMessage` on
  `CustomerAuthSession.session` keeps the Kratos object byte-for-byte. A
  `map[string]any` would have turned numbers into floats.
- **`CustomerCredentials.password`:** `writeOnly` was dropped. oapi-codegen
  turns writeOnly into an optional pointer.
- **Duplicate registration:** Kratos reports 4000007 in `ui.messages`, so the
  proxy returns it with field `form`, not `login`. The app shows it as a global
  message.
- **Kratos `show_verification_ui`:** this Kratos config does not return it
  for API registrations, so `verification_flow_id` is usually absent. The
  app then starts verification with the session's handle, as before.
- **Recovery for a legacy customer with an unbound vault row (transition
  only):** the handle is tried first, so the code may not be delivered.
  This is an edge case that only exists until migration is complete.

## Verification run (2026-10-04, local stack rebuilt)

| Command | Result |
| --- | --- |
| `go build ./... && go vet ./... && go vet -tags integration ./...` | clean |
| `go test -race ./...` | all packages ok |
| `go test -race -count=1 -tags integration ./...` | all packages ok (after the postgres test fix) |
| `node scripts/smoke.mjs` | all smoke checks passed, including the 7 PLX checks |
| `node scripts/seed-customers.mjs` | the 5 existing customers were found through the proxy (skipped as existing) |
| Manual: customer registered with the **old** build, signed in after the upgrade | 200, handle unchanged (PLX-NFR-05) |
| `flutter analyze` | no issues |
| `flutter test` | 162 passed |
| `flutter test -P integration` | 7 passed against the stack |
| `npm run typecheck` (admin-web) | clean |
