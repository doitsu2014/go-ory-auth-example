---
name: bugfix
depth: Minimal
keywords:
  - bug
  - fix
  - regression
  - hotfix
  - defect
  - broken
description: "Fast path from a reproduced defect to a verified fix"
skeleton: off
review_cap: advisory
guard_policy: relaxed
sensors: off
learnings: on
summary_confirmation: off
phases:
  - analyze
  - develop
  - launch
skip:
  - research-synthesis
  - feasibility-assessment
  - task-planning
  - refactor
  - test-plan
  - deployment
---

# bugfix scope

`bugfix` is a fast, evidence-driven path: reproduce, fix, verify, ship. The
design and planning ceremony is skipped; the review and validation ceremony
stays.

## Why these stages

Intent Capture becomes a defect statement with a reproduction. Requirements
Analysis is optional and self-skips when the defect is unambiguous. Code
Generation applies the fix; Code Review guards the change. Test Generation
adds a regression test; CI and release validation confirm the fix ships.

## Membership

Analyze: intent-capture, requirements-analysis.
Develop: code-generation, code-review.
Launch: test-generation, ci-pipeline, release-validation.

Everything else is SKIP.
