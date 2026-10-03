# Review Record — code-review (encrypted PII)

Reviewers: security-agent (design review before coding, then code review with
live probes on OpenBao, Postgres grants, API and logs), code-reviewer-agent
(Go + admin web + mobile correctness). The implementing developers fixed each
finding, and the lead re-ran all checks afterwards.

## Design review (before code) — 6 blocking, 11 advisory

All resolved in technical-spec §9. Highlights:
- free-text reveal reason replaced by `reason_code` + `ticket_ref`;
- lookup rate limits, with no email in lookup results;
- erasure ledger plus a backup accepted-risk entry;
- Transit `associated_data` binding;
- format byte added to the AAD;
- fixed-width masking;
- token revocation;
- corrected compliance mapping.

## Code review findings

| # | Sev | Area | Finding | Resolution |
| --- | --- | --- | --- | --- |
| C1 | Major | Go put/erase | Erase racing PUT returned 409/404 (not in contract) | Single retry of `keyFor`; tests |
| C2 | Major | Go DEK cache | Erased DEK could be re-cached by an in-flight read | Tombstones (2× TTL) block `Put`; test |
| S1 | Major | Go lookup | Self-declared phones let an attacker crowd the real owner out of 20 results | Fetch 21 → `truncated: true` + `pii_lookup_candidate_overflow`; contract + web warning |
| C3 | Minor | ledger | Ledger lives in the DB it repairs | Documented: production needs an audit copy outside the DB (log pipeline/SIEM) |
| C4 | Minor | rewrap | Skip check ignored KEK name | Compares name + version; conflict test |
| C5 | Minor | mobile | `ref` used after `await` on a disposed screen | `mounted` guards; tests |
| C6 | Minor | clients | Validation drift (runes, C1 controls, UTC age) | Aligned; tests |
| C7/C8 | Minor | Go | `reason_code` missing → wrong code; unknown props / bad date not field-mapped | `required`; strict body check middleware (`unknown_field`, field `invalid_format`) |
| C9 | Minor | web | Stale lookup results after a client-side error | Cleared; test |
| S2/S3 | Minor | Go lookup | Decrypted all fields of non-matches; quota per request | Phone-only first; quota weighted per candidate |
| S4 | Minor | Go masked | Masked reads unlimited | 300/min per actor |
| S5/S6 | Minor | OpenBao adapter | Followed redirects (token re-sent); all 400 = integrity | No redirects; 400 classified |
| S7 | Minor | config | http to OpenBao outside production | https unless local/test; `PII_OPENBAO_CA_FILE` |
| S8 | Minor | policy | Operator could set `exportable` etc. | `allowed_parameters` limited to `min_decryption_version`, `auto_rotate_period`; verified denial live |
| S9 | Minor | Go reveal | Existence probe before quota | Quota first |
| S10 | Minor | mobile | PII in OS snapshots | `FLAG_SECURE` (Android) + lifecycle cover (iOS) |
| S11 | Accepted | Go | Go strings can't be zeroed | Documented (DD-12, 08 §8.5) |
| H | Hardening | DB | Table-level UPDATE | Migration 0004 column-level grants; integration test (42501) |

## Verification after fixes (lead, 2026-10-03)

| Check | Result |
| --- | --- |
| Go vet (both tags), `test -race`, `test -race -tags integration -count=1` | pass (11 integration pkgs) |
| `make up` | all services healthy; OpenBao unsealed by `openbao-init` |
| PII survives `make down && make up` | pass |
| `node scripts/smoke.mjs` | 25/25 |
| Mobile analyze / test / integration | clean / 107 (+1 skipped tag) / 6/6 |
| Web lint / typecheck / test / build | pass (81 tests) |
| OpenAPI lint | 0 errors |

## Remaining (non-blocking)

- Kotlin `FLAG_SECURE` change is not compiled because there is no Android SDK.
- The iOS cover is a Flutter overlay, so a snapshot can be taken before it is drawn.
- Rate limits and the DEK cache are per replica.
- Admin PII checks run in Go e2e, not in the smoke script; the smoke admin stays at AAL1 without TOTP.
- The blind-index key can't be rotated (v1).

## Verdict

**Approve.** There are no open blocking or major findings. Merge needs the owner's review.
