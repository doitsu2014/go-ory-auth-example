# Code Generation Notes

## Verification (re-run by the lead on 2026-10-03, stack up)

| Unit | Command | Result |
| --- | --- | --- |
| U1 | `docker compose config -q`; `make infra-up` / `make up` | pass; all containers healthy |
| U2 | `npx @redocly/cli lint api/openapi/identity-service.v1.yaml` | valid (warnings only: tag descriptions, licence) |
| U3/U4 | `go build ./...`, `go vet ./...` (+ `-tags integration`) | pass |
| U3/U4 | `go test -race -count=1 ./...` | pass (httpapi, kratos, app, platform) |
| U3/U4 | `go test -race -count=1 -tags integration ./...` | pass (httpapi, keto, kratos, postgres, app, e2e, platform) |
| U3/U4 | `go generate` then diff of oapi output | stable |
| U3/U4 | `make seed` | printed a recovery link (agent run) |
| U5 | `pnpm lint`, `pnpm typecheck`, `pnpm test --run` (39), `pnpm build` | pass |
| U6 | `flutter analyze` | no issues |
| U6 | `flutter test` | 59 passed, 1 skipped (integration suite) |
| U6 | `flutter test -P integration` | 5/5 passed (registration, verification, settings, **recovery**, login + `/v1/me`, logout) |
| U7 | `node scripts/smoke.mjs` | 19/19 PASS |

Not run: `flutter build apk` (no Android SDK), Playwright E2E (no browsers installed), golangci-lint (not installed).

## Spike results

- **S1** Flutter + `ory_client` 1.22.22 works on Dart 3.14 dev; hand-built forms with Kratos messages by id.
- **S2** Kratos CORS for `http://localhost:5173` with credentials verified; preflight 204.
- **S3** Interrupting after-login webhook works on Kratos v26.2.0: webhook 403 + `{"messages":[{"instance_ptr":"#/","messages":[{id,text,type}]}]}` → Kratos fails the login with 400 and issues no session. Verified both directions.

## Kratos behaviour discovered (feed into docs)

- Native registration `continue_with` contains only `set_ory_session_token` (no `show_verification_ui`); clients start their own native verification flow.
- Native recovery needs `feature_flags.use_continue_with_transitions: true` to return `set_ory_session_token` + `show_settings_ui` (added to `kratos.yml.tmpl`; verified by smoke + mobile integration).
- An admin with TOTP doing password login gets 422 `browser_location_change_required` (to the aal2 step) with an AAL1 cookie.
- TOTP enrolment in settings upgrades the session to AAL2.
- `DELETE /admin/identities/{id}/sessions` returns 404 when there are no sessions (treated as success).
- Keto subject = subject set `User:<uuid>` with empty relation.

## Deviations (with reasons)

| Deviation | Reason |
| --- | --- |
| Hand-written Ory HTTP adapters (Go) | `kratos-client-go` lags v26; `keto-client-go` unmaintained |
| OpenAPI 3.0.3 instead of 3.1 | oapi-codegen support |
| Admin web: own Kratos node renderer, not `@ory/elements-react` | CSP control, message-id i18n, explicit §3.9 handling |
| Admin web: TS 5.9 / Vite 7 / RR 7 (not newest majors) | typescript-eslint compatibility |
| Mobile: hand-written models + dio client; no freezed / openapi-generator | no build_runner on dev SDK; Java not installed |
| Integration tests against live stack instead of testcontainers | faster to wire; testcontainers is a Launch follow-up |
| Admin with no role tuple reported as `support` | contract marks `role` required |
| 12 h admin cap → session revoked + 401 | owner's choice left open in docs |

## Known gaps (carried to Launch / Curate)

- Customer/admin lists can return short pages (Kratos can't filter by schema/state).
- Last-super_admin check is not concurrency-safe (no lock).
- golangci-lint/depguard config not added; Playwright E2E not run; no admins/audit page tests in web.
- Mobile: no inline re-auth on `session_refresh_required`; iOS ATS local exception documented only; no device run.
- `public/_headers` CSP uses placeholder origins.
