# Unit Breakdown

| Unit | Scope | Depends on | Status |
| --- | --- | --- | --- |
| U1 | Trace code: 4 parallel read-only traces | — | done |
| U2 | Customer auth pages F01–F05 | U1 | done |
| U3 | Self-service and cross-cutting pages F06–F08, F16 | U1 | done |
| U4 | Admin pages F09–F15 | U1 | done |
| U5 | Catalogue `docs/features/README.md` (context, use-case, catalogue, legend) | U2–U4 | done |
| U6 | Fix the 2 broken existing blocks, add pointers in 01/03/06/08/09/10 and `docs/README.md` | U5 | done |
| U7 | Verification: parse, cited paths, anchors, links | U2–U6 | done |
