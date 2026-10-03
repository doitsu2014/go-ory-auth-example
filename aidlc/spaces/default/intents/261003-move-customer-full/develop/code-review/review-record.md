# Review Record — code-review

Reviewers:
- security-agent: migration, crypto, leak paths, live read-only probes.
- code-reviewer-agent: correctness, contract, tests, docs.

Every finding was fixed by the implementing developers (N1 Go, N3 mobile) or by the lead (docs and specs), then re-verified.

## Findings and resolution

| # | Sev | Area | Finding | Resolution |
| --- | --- | --- | --- | --- |
| S-M1 | Major | Go `namemigration.go` | An erase committing between the erased-check and `keyFor` led to a new DEK, and the name was stored for an erased customer. Ledger replay doesn't catch it. | The store transaction re-checks `customer.pii.erased` after `SetName`, rolls back and strips only. A key created by this call is removed. `TestNameFR08_EraseBetweenDecideAndStore` |
| S-M2 | Major | Go `EraseMine` | Erasing with no PII record left no ledger entry, so a later migration stored a legacy Kratos name. | `customer.pii.erased` is appended when a name trait exists, then the trait is removed. A failed strip returns 503. Unit test and e2e `TestNameFR06_*` |
| S-m1 | Minor | Rollout | Unmigrated identities can't be disabled (Kratos validates the schema on PATCH). | Runbook in 08 §8.11 (migrate first, `--strip-invalid` if needed). Accepted risk in 06 §6.7 |
| S-m2 / C-m2 | Minor | PUT semantics | An outdated client omitting `name` clears it. | Rollout order: clients first, then a minimum app version, then the CLI. Accepted risk in 06 §6.7 |
| S-m3 / S-n2 | Minor | Migration | Invalid legacy names stay in Kratos; strip-only removals weren't audited. | `--strip-invalid`; `customer.pii.name_trait_removed` with a reason, hidden from machines; tests |
| S-n1 | Nit | Validation | Cf characters (zero-width, bidi) were accepted in names. | Rejected on the server (full Cf) and on mobile. Cf-only parts count as empty |
| S-n3 / C-n4 / C-n5 | Nit | Docs and spec | AAD label documented as `name_ct`; `test`+`remove` still described. | 0005 comment, tech spec, requirements and DD1/DD5 corrected |
| C-m1 | Minor | `dev` | `./dev migrate-names` exited 0 even when the run failed. | Returns the CLI status; only `cmd_up` downgrades it to a warning |
| C-m3 | Minor | Mobile profile | Fetched all PII onto an unprotected screen just to show the name. | Name row removed from the profile; the real name appears only on the protected personal-info screen (FR-09 revised) |
| C-n6 | Nit | Smoke | Masked-name check missing. | Dropped from spec §7, since admin calls need an AAL2 browser session. Covered by e2e and httpapi tests |

## Verification after fixes (lead, 2026-10-03)

| Check | Result |
| --- | --- |
| Go build, vet, `test -race` unit | pass (192 per developer; lead re-run: 11 packages ok, 0 FAIL) |
| Go `test -race -tags integration` | 227 pass (developer) |
| web lint, typecheck, test | pass, 114 tests |
| mobile analyze, test | 0 issues; 119 pass, 1 skipped (backend integration) |
| `./dev up` + `./dev smoke` | migration idempotent (all 0); smoke all pass |
| Live data | 32 customers, 0 with `traits.name`; 21 `customer_pii` rows, all with `name_ct` (format byte 0x01) |

## Verdict

**Approve.** No open blocking or major findings. Merge needs a human review (org rule).
