---
name: classic
depth: Standard
keywords: []
description: "Full lifecycle through all five phases with one human approval per stage"
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
  - curate
skip: []
---

# classic scope

`classic` is the implicit default scope — used when neither the user nor
`MY_AIDLC_DEFAULT_SCOPE` names one. It runs every phase with one human
approval per stage, advisory reviews, and the learnings ritual on.

Guard Policy defaults to relaxed: changed inputs are recorded and announced,
and plan approval is lowered only for undirected work. Human-turn authority
and the audit trail remain in force.

## Why these stages

Every phase participates. Analyze establishes the intent, requirements, and
feasibility. Ideate designs the system. Develop plans and implements units.
Launch tests, pipelines, deploys, and validates. Curate observes and improves.

Conditional stages (research synthesis, refactoring, incident analysis,
performance optimization) self-skip at runtime after a named-scope check, with
the reason recorded.

## Membership

All five phases, all 21 stages. Conditional stages remain CONDITIONAL and
decide at their own condition check.
