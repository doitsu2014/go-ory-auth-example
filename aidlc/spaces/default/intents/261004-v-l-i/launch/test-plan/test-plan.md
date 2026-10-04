# Test Plan

| Test | Requirement | Method | Pass criterion |
| --- | --- | --- | --- |
| T1 Mermaid parse | R7 | `check.mjs` (mermaid 11 `parse` under jsdom) on `docs/**/*.md` | 0 failures |
| T2 Diagram presence | R2, R3 | grep each `docs/features/F*.md` | ≥1 `flowchart` and ≥1 `sequenceDiagram` per page |
| T3 Cited paths | R4 | Script: each cited path exists and each line ≤ file length | 0 missing |
| T4 Anchor accuracy | R4 | Spot-check 20 `file:line` anchors against the named function | 20/20 |
| T5 Links | R1, R8 | Relative `.md` links resolve | 0 broken |
| T6 Catalogue completeness | R1 | README links F01–F16 | 16/16 |
| T7 Fact check | R4–R6 | Independent reviewer agent | All "wrong" findings fixed |
| T8 Scope | C1 | `git diff --stat` | Only `docs/` and `aidlc/` touched |
