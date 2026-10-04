# Release validation

Verdict: **ready for human review**. Nothing is committed yet; per org
rules, a human reviews before merge.

## Requirements

| Req | Status | Evidence |
| --- | --- | --- |
| PLX-FR-01 login proxy | pass | unit, HTTP, smoke, Flutter integration |
| PLX-FR-02 registration proxy, random handle | pass | smoke: opaque handle, duplicate gives 4000007 |
| PLX-FR-03 recovery (sealed ref + code endpoint) | pass | smoke: no flow id; forged id gives 422; wrong code gives 4060006; grant, new password, login |
| PLX-FR-04 lookup_key / migration 0007 | pass | postgres integration test; the old customer's handle is unchanged |
| PLX-FR-05 resolve endpoint removed | pass | 404 (smoke, HTTP test) |
| PLX-FR-06 message ids only | pass | adapter test (node value dropped), smoke (no `login.invalid` in rejections) |
| PLX-FR-07 legacy transition | pass | unit tests (login, recovery, stray row) |
| PLX-FR-08 mobile | pass | 163 unit tests and 7 integration tests |
| PLX-FR-09 admin lookup | pass | unit tests and the e2e PII integration test |
| PLX-NFR-01..05 | pass | see test plan; account limit verified under concurrency |

## CI-equivalent run (local)

- `make generate`: no drift.
- `gofmt`, `make vet`, `make test`, `make build`: pass.
- `make test-integration`: pass.
- `make login-migrate`: 57 scanned, 0 failed.
- `node scripts/smoke.mjs`: all checks pass.
- `dart format`, `flutter analyze`, `flutter test`: pass.
- admin-web `generate:api`, `typecheck`, `lint`, `test`: pass.

## Accepted residual risks (documented in ADR-0014 and 06-security)

- Registration still reveals that an account exists, as native Kratos does.
- Matching the timing of unknown and known handles depends on how Kratos is tuned (follow-up).
- The account lockout window is at most 15 minutes (follow-up: account+IP key or CAPTCHA).
- The Kratos public self-service API is still reachable; ingress hardening is a follow-up.
- Pre-0007 handles are HMAC-derived; rotating them is a follow-up.
- Passwords pass through identity-service memory (controls: strict bodies, no body logging, value-free errors).
