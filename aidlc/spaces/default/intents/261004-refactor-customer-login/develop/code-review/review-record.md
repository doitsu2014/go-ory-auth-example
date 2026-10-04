# Code review record

The independent reviewer (code-reviewer-agent) read the whole diff,
excluding docs and aidlc. Every finding was checked against the code, and
#1 was also confirmed on the running stack.

| # | Severity | Finding | Resolution |
| --- | --- | --- | --- |
| 1 | **Blocking** | `POST /v1/auth/recovery` returned the Kratos flow id. `GET /self-service/recovery/flows?id=` is public, and its `email` node shows the handle. This re-opened the oracle (handle disclosure, enumeration by comparing handles, direct Kratos login bypassing the limits). **Confirmed on the stack.** | Fixed. The response is now `recovery_id`: the flow id sealed with the login KEK (AEAD, AD `identity-service/recovery-flow/v1`). New `POST /v1/auth/recovery/code` submits the code server side and returns `{session_token, settings_flow_id}`. A forged id gets 422, an expired flow 410 `auth_flow_expired`. The app no longer sends anything to a recovery flow. Smoke checks were added |
| 2 | Major | The per-account limit used `Peek` before the Kratos call, so concurrent guesses all passed | Fixed. `Allow` now records the attempt before the call. Test with 40 concurrent attempts: exactly 10 reach Kratos |
| 3 | Major | Recovery for a legacy customer with a stray unbound row used the unused handle, so no code was sent (an attacker could repeat this daily) | Fixed. `recoveryTarget` asks the Kratos admin API whether the handle is in use; if not, it uses the legacy email. Tested |
| 4 | Minor | Registration reveals that an account exists (4000007), as native Kratos does | Accepted and documented. PLX-NFR-03 covers login and recovery only |
| 5 | Minor | Timing: Kratos's unknown-identifier delay vs bcrypt; a second call for unbound rows during the transition | Accepted and documented as a follow-up (tune Kratos, measure) |
| — | Note | The account limit allows a lockout for up to 15 minutes | Accepted and documented. Follow-up: a combined account+IP key or a CAPTCHA |
| — | Note | A handle holder can call Kratos `/self-service/login/api` directly | Documented. Production ingress should restrict the Kratos self-service endpoints (follow-up) |

The reviewer checked these and found them fine: the claim race (`ON CONFLICT`
plus re-read), value-free errors, message-id-only rejections, `no-store`,
forwarded header safety, strict bodies, the 0007 backfill, and the mobile
screens.

Re-verification after the fixes: Go unit and integration tests, smoke (all
checks), Flutter unit (163) and integration (7), and the admin-web typecheck
all pass.
