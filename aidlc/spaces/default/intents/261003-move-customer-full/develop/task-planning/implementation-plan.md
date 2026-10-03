# Implementation Plan

1. N0 contract (lead) → N1, N2, N3 in parallel (three developer agents, disjoint directories).
2. N4 docs by the lead after N1 lands.
3. Review: code-reviewer (Go + clients) and security-agent (migration ordering, leaks, erasure ledger), fixes sent back to the implementing agents.
4. Verification: Go unit + integration, web lint/typecheck/test/build, mobile analyze/test, `./dev up` (runs the migration), `./dev smoke`, raw DB + Kratos checks.
