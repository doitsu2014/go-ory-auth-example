---
name: mvp
depth: Standard
keywords:
  - mvp
  - prototype
  - launch fast
  - ship fast
  - first version
description: "Analyze through Launch with the minimum design needed to ship"
skeleton: on
review_cap: advisory
guard_policy: relaxed
sensors: on
learnings: on
summary_confirmation: off
phases:
  - analyze
  - ideate
  - develop
  - launch
skip:
  - design-patterns
  - api-contract
  - refactor
  - deployment
---

# mvp scope

`mvp` gets a first version in front of users with the minimum design needed to
ship safely. It favours a walking skeleton and a thin design pass over a
complete specification.

## Why these stages

Architecture Design and Technical Specification stay: even a first version
needs boundaries. Pattern selection and formal API contracts are deferred.
Deployment automation is skipped when the first version ships manually or on
a managed platform.

## Membership

Analyze: all four stages.
Ideate: architecture-design, technical-spec.
Develop: task-planning, code-generation, code-review.
Launch: test-plan, test-generation, ci-pipeline, release-validation.

Everything else is SKIP.
