---
name: feature
depth: Standard
keywords:
  - feature
  - add
  - enhance
  - capability
  - story
description: "Analyze through Launch for a new capability, with a full design pass"
skeleton: off
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
  - research-synthesis
---

# feature scope

`feature` is the default profile for adding a capability to an existing
system. It runs the full design pass and stops before continuous curation.

## Why these stages

Research Synthesis self-selects out because the solution space is usually
known for an incremental feature; promote it when it is not. Curate is out of
scope: a feature hand-off ends at a validated release. Incident analysis and
performance work are separate intents.

## Membership

Analyze: intent-capture, requirements-analysis, feasibility-assessment.
Ideate: all four stages.
Develop: task-planning, code-generation, code-review, refactor.
Launch: test-plan, test-generation, ci-pipeline, deployment, release-validation.

Curate and research-synthesis are SKIP.
