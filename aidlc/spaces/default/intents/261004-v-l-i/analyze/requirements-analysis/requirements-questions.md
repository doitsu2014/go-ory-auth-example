# Requirements Analysis — Questions

Mode: YOLO (question budget 0). Each question took the recommended answer.

## Q1. How fine-grained is "feature"?

- A. One page per user-visible capability. Endpoints that share a flow share
  a page (for example enable/disable/revoke sessions). *(recommended)*
- B. One page per endpoint.
- C. One page per app.

[Answer]: A — Recommendation.

## Q2. What does "functional diagram" mean here?

- A. At system level, a use-case style diagram of actors and features. Per
  feature, a flowchart of the decision logic (validation, authz, branches,
  outcomes). *(recommended)*
- B. Only component diagrams.

[Answer]: A — Recommendation.

## Q3. How much detail goes in the sequence diagrams?

- A. Real participants, HTTP method and path, key fields, Keto relation
  tuples, DB tables, audit event names, and the main error branches as
  `alt`/`opt` blocks. *(recommended)*
- B. Only the happy path, at a high level.

[Answer]: A — Recommendation.

## Q4. How do we verify the diagrams?

- A. Parse every Mermaid block with mermaid-cli (or an equivalent parser). Each
  step must cite a `file:line` that exists. *(recommended)*
- B. Visual review only.

[Answer]: A — Recommendation.
