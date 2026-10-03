# Diary — code-generation

## Deviation
- 2026-10-03: Three parallel tracks (Go, React, Flutter) against the OpenAPI contract; lead owns deploy/ and integration.

## Open question
- Do we want testcontainers in CI, or a compose-based integration job?

## Candidate learnings (YOLO: not admitted, for owner review)
- Kratos native recovery needs `feature_flags.use_continue_with_transitions: true`. (project)
- Kratos native registration does not emit `show_verification_ui`; mobile starts the verification flow itself. (project)
