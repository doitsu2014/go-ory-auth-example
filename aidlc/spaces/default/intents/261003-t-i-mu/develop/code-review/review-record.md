# Review Record — code-review

Reviewers: code-reviewer-agent (Go correctness), security-agent (Go + Ory
config, live probes), code-reviewer-agent (admin web + mobile). Every finding
was sent back to the implementing developer and fixed. The lead re-ran all
verification afterwards.

## Findings and resolution

| # | Sev | Area | Finding | Resolution |
| --- | --- | --- | --- | --- |
| S1 | **Blocking** | Go `app/admingate.go` | MFA enrolment deadline bypass: TOTP enrolled directly at Kratos after the deadline → AAL2 passes the gate (confirmed live) | Gate loads the TOTP credential on every admin request; late enrolment → deactivate + revoke + 403; periodic sweeper; `TestS1_E2E_LateTOTPEnrolmentIsBlocked`, `TestFR06_MFASweeper` |
| S2 | Major | compose | `serve` held the migrator DSN | One-shot `identity-migrate` service; `serve` has DML role only (verified: 0 MIGRATE_* vars in container) |
| C1/C4 | Major | Go invite/bootstrap | Orphaned, unaudited admin when the tx fails; idempotency race | Reserve key → Ory side effects → detached tx → email; compensation; `orphaned_admin` log |
| C2 | Major | Go mutations | Mutation without an audit row | Single strategy (`app/mutation.go`): audit INSERT → Ory call → COMMIT |
| C3/S3 | Major | Go role change | Last super_admin race / inactive admins counted | `pg_advisory_xact_lock` + only active identities counted |
| W1 | Major | web `useKratosFlow` | Retry stuck in the error state; lost `aal`/`return_to` | `retry()` + `FlowErrorView`; tests |
| M1 | Major | mobile `flow_form` | Kratos 401 did not sign the user out | `handleUnauthorized()` on submit/load; tests |
| M2 | Major | mobile settings | Password change broke after the privileged window | Inline refresh re-auth + retry; tests |
| — | Minor ×20 | all | Email case, page_size 422, cache epoch, config validation, NUL bytes, guard fail-closed 404, XFF hops, request-id, SMTP log PII, migration timeout, indexes (0002), web logout/invalidations/sourcemaps/i18n errors, mobile stale-token 401, banner clear, release https, recovery session revoke | Fixed |
| S6 | Minor | Kratos config | Browser registration/recovery produce sessions without the login guard | Accepted risk recorded in 06-security §6.7 with owner |
| — | Nit | postgres init | Passwords interpolated into SQL | psql `:'var'` quoting; verified in a throwaway container with a quoted password |

## Verification after fixes (lead, 2026-10-03)

| Check | Result |
| --- | --- |
| Go `build`, `vet` (+integration tag) | pass |
| Go `test -race` unit | pass (4 pkgs) |
| Go `test -race -tags integration` | pass (7 pkgs incl. e2e) |
| `make up` | identity-service healthy, identity-migrate exited 0 |
| web lint / typecheck / test (54) / build (no prod sourcemaps) | pass |
| mobile analyze / test (74) / integration (5/5) | pass |
| `node scripts/smoke.mjs` | 19/19 pass |

## Remaining (non-blocking, carried forward)

Playwright E2E not run (no browsers); no Android SDK build; golangci-lint/
depguard not configured; Kratos list can't filter by schema (short pages);
per-replica session cache (≤ 30 s lag); web cache-invalidation tests missing.

## Verdict

**Approve** — no open blocking or major findings. Merge needs the owner's review (org rule: humans own merges).
