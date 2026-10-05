# Release Validation

| Req | Acceptance criterion | Evidence | Status |
| --- | --- | --- | --- |
| R1 | Catalogue with use-case and context diagrams, linking F01–F16 | `docs/features/README.md`, T6 16/16 | Pass |
| R2 | Functional flowchart on every page | T2 | Pass |
| R3 | Sequence diagram(s) on every page | T2 (51 new blocks in total) | Pass |
| R4 | Matches current code, with paths that exist | T3, T4, and T7 (all wrong findings fixed) | Pass |
| R5 | Error branches shown | `alt`/`opt` blocks and error nodes on every page, plus "Errors and outcomes" tables | Pass |
| R6 | Security facts visible | Keto relations, audit actions, OpenBao encrypt/decrypt and HMAC steps, and transactions (grey `rect`) are labelled | Pass |
| R7 | Mermaid valid | T1 70/70 | Pass |
| R8 | Discoverable | `docs/README.md` row 16 and note, plus pointers in 01/03/06/08/09/10 | Pass |
| C1 | Docs only | T8 | Pass |

**Verdict: release-ready.** The change has not been committed; a human
reviews it before merge (org rule).

## Known limitations

- The diagrams describe behaviour on 2026-10-04. Re-run T1, T3 and T5 when
  the code changes.
- The architecture docs still contain their older diagrams. They now point
  to the feature pages, but their narrative was not rewritten (out of scope).
