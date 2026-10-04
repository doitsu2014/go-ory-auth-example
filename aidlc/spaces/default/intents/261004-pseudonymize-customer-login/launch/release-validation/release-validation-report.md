# Release Validation Report — Pseudonymous customer login identifiers

**Release candidate:**

- Base commit: `db79274`, branch `doitsu2014/sandworm`. The change is
  uncommitted; working-tree fingerprint `cd9bdbef9954` (sha1 of the diff plus
  new files, excluding `aidlc/`).
- Two later edits are covered by the runs below: the Flutter
  integration-test filter and the `CLEANUP_SLEEP` Makefile knob.
- Validated on 2026-10-04 against the local compose stack rebuilt from the
  candidate (`make up`).

## 1. Full verification (re-run on the candidate)

| Gate | Result |
| --- | --- |
| Go gofmt / vet (+integration tag) / unit (`-race`) / build | clean / ok / 13 packages ok / ok |
| Go integration + e2e (`make test-integration`) | ok (all packages) |
| Generated code drift (oapi, sqlc, `schema.d.ts`) | none |
| Smoke (`scripts/smoke.mjs`) | **47 PASS / 0 FAIL** |
| Mobile analyze / unit+widget | no issues / 174 passed, 1 skipped (integration-tagged) |
| Mobile integration against the stack | 7/7, stable across 4 consecutive runs after fix L5 |
| Admin web typecheck / lint / test / build | ok / ok / 130 passed / ok |
| Migration on existing data | `scanned=32 migrated=32 failed=0`; re-run `migrated=0 skipped=33 failed=0` |
| Courier scrub (`make kratos-scrub`, phase transition) | ran; `courier_messages_deleted 0` (retention 7 days) |
| Resolve latency (local, 15 calls) | p50 1.1 ms, p95 1.8 ms, max 20.2 ms (target < 60 ms) |

Defect found during validation (fixed):

- **L5:** a flaky Flutter integration test. The recovery step picked up a
  verification code that arrived late through the http courier (Kratos
  4060006). Mailpit lookups now match the message template
  (recovery / verification).

## 2. Requirement coverage (Must)

Every Must requirement (PLI-FR-01..16 and 19, PLI-NFR-01..08, 10 and 11) has
passing verification. Traceability is in
`launch/test-generation/test-suite.md`.

| Not fully covered | Decision | Owner |
| --- | --- | --- |
| FR-14 legacy scrub in phase **complete** | Runbook step 5 (≥ 24 h after migration). Not run locally: the local stack stays on the transition config | platform lead |
| FR-17 (Should) account-deletion hook | Covered by the purge orphan path (`customer.login.erased`); no explicit admin delete endpoint exists | identity-service maintainers |
| NFR-09 (Should) `/v1/me` p95 | Not measured separately; covered functionally. Resolve measured (above) | identity-service maintainers |
| NFR-12 minimum app version | Process/runbook item (deployment pre-condition 1) | mobile lead |

## 3. Operational readiness

| Item | Status |
| --- | --- |
| Rollback | Steps 1–2: previous images/config (not exercised). Steps 3–5: **roll forward only**; the reverse migration is a documented, untested emergency procedure. The go/no-go gate sits after a 24 h soak on the transition config |
| Alerts | `deploy/observability/login-alerts.yml`: courier drops and errors, SMS budget, unbound rows, pre-registration errors |
| First hour on-call watch | `identity_courier_dispatch_total{outcome="error"}` = 0; `identity_login_preregistration_total{result="error"}` = 0; drops by reason (spikes in `unresolved`/`unbindable` mean a binding defect, in `over_quota` abuse); Kratos `courier_messages` queued count; `/readyz`; customer sign-in success rate |
| Key safety | Pseudonym key `deletion_allowed=false`; OpenBao snapshot + restore drill is a deployment pre-condition |
| Docs | ADR-0013; 01/02/03/04/05/06/08; API guide and changelog; runbook (08 §8.12, deployment-runbook.md) |

## 4. Known issues and follow-ups

1. `TRUSTED_PROXY_CIDRS` (SEC-C09): accepted risk with an owner.
2. CLI `pii rekey-logins` (SEC-C12): runbook exists, CLI is a follow-up.
3. Any future registration method must carry the pre-registration hook
   (SEC-C07).
4. Mobile dependency `crypto: ^3.0.7` (promoted from transitive, same
   version) needs human sign-off under the dependency-change rule.
5. CI workflow `.github/workflows/ci.yml` is new. Its first GitHub run
   happens after push; the commands were all verified locally.
6. The local stack now runs this branch. The other checkout's older
   identity-service is incompatible with the migrated customers.

## 5. Recommendation

**Go, with the known issues above.**

- Every must-have requirement passes on the real stack.
- All blocking and major review findings are fixed.
- Operational controls are documented.

The one-way nature of the migration is handled by the phased runbook and the
soak gate.

## 6. Human release gate

**Pending: the release decision belongs to the owner.** The model recommends
and does not decide. Nothing has been committed, pushed or deployed beyond
the local stack.
