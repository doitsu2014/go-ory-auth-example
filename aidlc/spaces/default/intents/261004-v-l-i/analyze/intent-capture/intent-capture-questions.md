# Intent Capture — Questions

Mode: YOLO (question budget 0). Each question below was answered with the
recommended option and recorded as a Recommendation.

## Q1. What problem does this solve, and for whom?

- A. The diagrams are spread across ten architecture docs, and some no longer
  match the code after ADR-0013/0014. The team needs one current set of
  diagrams per feature. *(recommended)*
- B. Only the auth flows need redrawing.
- X. Other

[Answer]: A — Recommendation.

## Q2. What does success look like?

- A. Each feature has its own page with a functional diagram (flowchart) and at
  least one sequence diagram. Every step is traced to code (`file:line`), and an
  index links the pages. *(recommended)*
- B. A single large diagram for the whole system.
- X. Other

[Answer]: A — Recommendation.

## Q3. Format and location?

- A. Mermaid in Markdown under `docs/features/`, in English to match the
  existing docs, linked from `docs/README.md`. *(recommended)*
- B. PlantUML or image files.
- X. Other

[Answer]: A — Recommendation.

## Q4. What is out of scope?

- A. Code, config and API changes. The existing architecture docs keep their
  diagrams, but stale ones get a pointer to the new pages. *(recommended)*
- B. Rewrite all of `docs/architecture/`.
- X. Other

[Answer]: A — Recommendation.

## Q5. What if we do nothing?

The diagrams keep drifting from the code. Reviewers and operators then reason
from flows that no longer exist, for example plaintext email in Kratos traits
before ADR-0013.
