# Code Review Record — Pseudonymous customer login identifiers

Three independent reviews ran on the full working-tree diff (read-only):

| Review | Reviewer | Scope | Verdict |
| --- | --- | --- | --- |
| R-GO | code-reviewer-agent | services/identity-service, OpenAPI, deploy/, Makefile, dev, scripts | Approve with changes (0 blocking, 1 major) |
| R-SEC | security-agent | implementation vs. A1–A14 + client caches | 1 blocking, 2 major, 11 minor |
| R-CLI | code-reviewer-agent | apps/mobile, apps/admin-web | Approve with changes (0 blocking, 1 major) |

## Findings and disposition

### Blocking / major

| ID | Sev | Finding | Fix | Status |
| --- | --- | --- | --- | --- |
| SEC-C01 | blocking | Kratos transport errors embedded `*url.Error` text, i.e. the full URL incl. `credentials_identifier=<email>` (legacy collision check, transition lookups) → logs | `transportCause()` strips the URL (timeout → "timeout", else the cause); test `TestSECC01_TransportErrorsCarryNoQuery` | fixed |
| SEC-C02 | major | SMS budget + per-recipient quota in process memory: reset on restart, multiplied by replicas | Counted in `courier_dispatch` (new columns `channel`, `country`, `recipient_key` = keyed hash; partial indexes); only **delivered** messages count | fixed |
| R-GO F1 | major | Quota charged before send; a transient outage + Kratos retries exhausted the 5/h quota and later dropped the code | Same fix: quota = delivered rows only; test `TestPLIA11_TransientOutageNeverDrainsQuota` (8 failures then success) | fixed |
| SEC-C03 | major | Global insert cap (120/min) refused every sign-up, charged even for existing rows | Charged only on a real insert and **alert-only** (`login_insert_rate_high`); new aggregate per-/24 (IPv4) and /48 (IPv6) limiter `LOGIN_RESOLVE_NET_RATE` (default 300/1m, 5000/24h); tests | fixed |
| R-CLI F1 | major | Mobile sign-up resolved on every submit (CGNAT + 5/min registration limit) | Repository keeps the pseudonym per (flowId, input), reuses it after a Kratos 400, and resolves again on an input or flow change; 2 tests | fixed |

### Minor / nit

| ID | Finding | Fix | Status |
| --- | --- | --- | --- |
| SEC-C04 / R-GO F3 | Per-country SMS cap equalled the global one; Peek+Allow not atomic | `SMS_COUNTRY_DAILY_BUDGET`; DB counts (overshoot bounded by in-flight messages, documented); test `TestPLIA3_CountryBudgetIndependent` | fixed |
| SEC-C05 / R-GO F2 | Permanent provider failures (invalid recipient, SMTP RCPT 5xx, SMS 4xx, rebind conflict) answered 5xx → 10 Kratos retries | `app.ErrPermanentDelivery` from mailer/SMS adapters → drop `provider_rejected` (204); rebind conflict → `ErrDataIntegrity` → drop `unbindable`; test `TestPLIA11_PermanentRejectionIsDropped` | fixed |
| SEC-C06 | `:keep_days` unquoted in psql | `(:'keep_days')::int`; documented other legacy plaintext (flows, code rows) and `KEEP_LAST` override | fixed |
| SEC-C07 | Pre-registration hook only on the password method | Guard comment in `kratos.yml.tmpl` (any new registration method must add the hook); only password is enabled | accepted (documented) |
| SEC-C08 | Pre-registration allowed / courier acked when the service was not wired | Both 503 (fail closed) | fixed |
| SEC-C09 | XFF trusted by hop count | Accepted risk in 06-security §6.7 with owner; follow-up `TRUSTED_PROXY_CIDRS`; aggregate net limiter mitigates | accepted |
| SEC-C10 | `purge --older-than` could be < 24 h | `MinPurgeAge` = 24 h enforced; test | fixed |
| SEC-C11 | No alert rules | `deploy/observability/login-alerts.yml` (drops, errors, SMS budget, unbound rows, pre-registration errors) | fixed |
| SEC-C12 | No re-key procedure | Runbook 08 §8.12; `init.sh` sets `deletion_allowed=false` explicitly; CLI `pii rekey-logins` = follow-up with owner | fixed (runbook) / follow-up (CLI) |
| SEC-C13 / R-CLI F2 | Mobile cache key unsalted sha256; not cleared on logout | Per-install 32-byte HMAC key in secure storage; `login_id.*` cleared on logout; cached value validated against the pseudonym pattern; tests | fixed |
| SEC-C14 | Audit action name drift | Docs aligned to `customer.login.lookup` (D4) | fixed |
| R-GO F4 | Possible nil deref in `bind` when a row is inserted between Bind and Get | Retry once; else `ErrDataIntegrity` | fixed |
| R-GO F5 | Re-bind replaced any binding | Query takes `stale_identity`: replaces exactly that binding; integration test | fixed |
| R-GO F6 | One conflict aborted the whole purge; dry-run counted one page | Per-row errors counted (`Conflicts`) and logged; paging continues in dry-run | fixed |
| R-GO F7 | Scrub could leave plaintext flows when run < 24 h after migration | `KEEP_LAST` override + documented timing | fixed |
| R-GO F8 | `*_invalid` templates dropped (contract allowed them) | Recorded as deviation D8 (safer; Kratos never sends them with `notify_unknown_recipients: false`) | accepted |
| R-GO F9 | Rewrap re-sealed every row | Skips rows already at the newest version learned from the first seal | fixed |
| R-GO F10 | `.env` append without trailing newline | Newline guard in Makefile and `dev` | fixed |
| R-GO F11 | Local seed + smoke hit the 5/min registration limit | Local compose sets `LOGIN_REGISTER_RATE=60/1m,1000/24h` | fixed |
| R-CLI F3 | Retry-After never sent by the server | Server sets `Retry-After: 60` on every 429 (test). The client shows seconds, minutes or hours, rounded up (vi/en tests) | fixed |
| R-CLI F4 | core→feature import | `login_input.dart` moved to `lib/core/identity/` | fixed |
| R-CLI F5 | Empty identifier sent on verify | `UnauthenticatedFailure` (session wiped, back to sign-in); repository guard too; tests | fixed |
| R-CLI F6 | Stale `?email=` stays in the address bar | Removed with `replace` (history REPLACE); test | fixed |
| R-CLI F7 | Stale lookup results while typing | Cleared on edit; late responses dropped; test | fixed |
| R-CLI F8 | Stale local format error | Cleared on field change (3 screens); tests | fixed |
| R-CLI note | Login cache not cleared by the local wipe after a 401 (`forget()`), only by logout | Accepted: a 401 keeps the same user on the device; logout is the user's explicit act | accepted |

## Verification after fixes (Go)

`cd services/identity-service && go build ./... && go vet ./... && go vet -tags integration ./... && go test ./...` → all packages ok.

## Verification after fixes (clients)

- Mobile: `flutter gen-l10n`; `dart format lib test` (0 changes); `flutter analyze` (no issues); `flutter test` → 174 passed, 1 skipped (integration, needs the redeployed stack), 0 failed.
- Admin web: `pnpm typecheck`, `pnpm lint`, `pnpm test --run` (14 files, 130 tests), `pnpm build` → all pass.

## Verdict

**Approved (auto, yolo) with the accepted items above.**

- Every blocking and major finding is fixed and has a test.
- Accepted risks carry owners in 06-security §6.7.
- Follow-ups:
  - `TRUSTED_PROXY_CIDRS` (SEC-C09);
  - CLI `pii rekey-logins` (SEC-C12);
  - a pre-registration hook for any future registration method (SEC-C07).
- Dependency change pending human sign-off (org rule): mobile `crypto: ^3.0.7`, promoted from a transitive dependency at the same version.
