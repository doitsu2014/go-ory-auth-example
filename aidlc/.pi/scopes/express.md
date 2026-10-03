---
name: express
depth: Minimal
keywords:
  - express
  - quick
  - lightweight
  - small
  - tiny
description: "Lightest run: requirements to deploy, no design pass, no reviewers"
skeleton: off
review_cap: none
guard_policy: off
sensors: off
learnings: off
summary_confirmation: off
question_budget:
  min: 0
  max: 3
phases:
  - analyze
  - develop
  - launch
skip:
  - research-synthesis
  - feasibility-assessment
  - code-review
  - release-validation
---

# express scope

`express` is the lightest useful run. It follows a straight line from
requirements to code, test, and deploy without a design pass or reviewers.

Guard Policy defaults to off: changed inputs are recorded and announced rather
than reopening approval. Human presence stays up — the approval gates still
fire.

## Why these stages

Intent Capture and Requirements Analysis establish the contract. Task
Planning, Code Generation, and Refactor implement it. The Launch tail tests,
wires CI, deploys, and (when present) validates. Ideate and Curate are skipped
entirely.

`code-review` is skipped as a separate stage, but Code Generation still
requires a human-approved diff before completion.

## Membership

Analyze: intent-capture, requirements-analysis.
Develop: task-planning, code-generation, refactor.
Launch: test-plan, test-generation, ci-pipeline, deployment, release-validation.

Everything else is SKIP.
