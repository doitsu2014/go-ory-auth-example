# Diary — architecture-design

## Deviation
- 2026-10-03: The normative architecture lives in `docs/` (the owner's explicit deliverable). The stage artifact is an index plus summary, so the two copies can't drift.

## Tradeoff
- 2026-10-03: Parent-domain cookie on a dedicated apex chosen over host-only `/.ory` routing (simpler Kratos base_url for mobile and web); risk accepted.

## Open question
- Spike S3: does the pinned self-hosted Kratos support interrupting after-login webhooks with `ctx.flow.type`?

## Candidate learnings (YOLO: not admitted, for owner review)
- Kratos config: env overrides have no product prefix, and YAML has no `${}` expansion. (project scope)
- Kratos admin-created recovery codes aren't emailed. Invitations need our own mailer. (project scope)
