# Review Record

Reviewer: an independent code-reviewer agent. It was read-only and
fact-checked every diagram against the code. All 9 priority pages and the
README were read in full. The rest were skimmed and spot-checked.

## Findings and resolution

| # | Severity | Page | Finding | Resolution |
| --- | --- | --- | --- | --- |
| 1 | Wrong | F03 | The after-login hook was drawn before the credential check. Kratos runs `login.after` hooks only after the password is accepted, so 4000001 ends the loop. | Fixed: credentials first, then the hook inside `opt password accepted` |
| 2 | Wrong | F03 | Legacy email was shown as a candidate only when there is no row. The code also appends it when the row is unbound (`customerauth.go:180`). | Fixed (verified in code) |
| 3 | Wrong | README | Context diagram: JWKS was attributed to Hydra admin, and the `:8081 → Kratos admin` edge was missing. | Fixed |
| 4 | Wrong | F01 | The password policy was drawn after the pre-registration hook. Kratos validates the password before the after-hooks. | Fixed: policy, then hook, then uniqueness |
| 5 | Wrong | F08 | Erase: the no-op path skipped the 503 check, and a failed `RemoveTraitName` was missing. | Fixed |
| 6 | Misleading | F11 | Disable can partly apply: the PATCH succeeds, the revoke fails, and the audit is rolled back. | Added a branch |
| 7 | Misleading | F06 | The public plane matches route and policy before checking the client IP (`middleware.go:298`). | Fixed (verified) |
| 8 | Misleading | F14 | A Hydra 409 or other 4xx does not trigger a compensating DELETE. | Split the branches |
| 9 | Misleading | F15 | The failure limiter also counts the 401s and 400s that happen before verification. | Fixed |
| 10 | Minor | F01 | Pre-registration step order, and `email` in the payload | Fixed |
| 11 | Minor | F01 | Dropped courier messages are marked too, and the plaintext-recipient path | Fixed |
| 12 | Minor | F04 | Target selection for an unbound row when the phase is complete or the login is a phone | Fixed |
| 13 | Minor | F06 | The admin plane rejects bearer and `X-Session-Token` | Fixed |
| 14 | Minor | F16 | `readyz` targets; DB edges for the migrations | Fixed |
| 15 | Minor | README | DB label table list | Fixed |

Pages confirmed accurate: F09, F10, F12, F13. F07, F11 and F15 were accurate
apart from the items above.

After the fixes, `check.mjs` reports **70/70 Mermaid blocks parse** across
`docs/`.

## Security observations (no change, out of scope)

See code-generation-notes items 1–3: the existing-account signal on
registration, refresh login bypassing identity-service's limits, and
per-replica rate limiters.
