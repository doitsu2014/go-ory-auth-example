# Test Suite — results (2026-10-04)

| Test | Result |
| --- | --- |
| T1 Mermaid parse (mermaid 11 + jsdom, `docs/**`) | **70/70 blocks parse**. Before: 17/19, because 2 existing blocks were broken. |
| T2 Diagram presence | 16/16 pages have ≥1 flowchart and ≥1 sequenceDiagram |
| T3 Cited paths and line ranges | 118 refs, 0 missing, 0 out of range |
| T4 Anchor spot-check | 20/20 anchors land on the named function |
| T5 Relative links | 0 broken across features, architecture, and the index |
| T6 Catalogue | 16/16 feature pages linked |
| T7 Independent fact-check | 15 findings: 5 wrong, 4 misleading, 6 minor. All fixed. |
| T8 Scope | Only `docs/` and `aidlc/` changed |

## Re-run

The parser script is kept in the session scratchpad as `check.mjs`. It loads
`mermaid` under `jsdom` and calls `mermaid.parse` on each ```` ```mermaid ````
block. The steps to re-run it are in `docs/features/README.md` under
"Checking the diagrams".
