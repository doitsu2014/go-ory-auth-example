---
name: poc
depth: Minimal
keywords:
  - poc
  - proof of concept
  - spike
  - experiment
  - prototype
  - validate idea
description: "Prove or disprove a technical assumption, then stop"
skeleton: on
review_cap: none
guard_policy: off
sensors: off
learnings: on
summary_confirmation: off
question_budget:
  min: 1
  max: 3
phases:
  - analyze
  - develop
skip:
  - research-synthesis
  - code-review
  - refactor
  - task-planning
---

# poc scope

`poc` proves or disproves a single technical assumption fast. The output is a
finding, not a product. Do not carry proof-of-concept code to production.

## Why these stages

Intent Capture frames the assumption and the falsification test. Requirements
Analysis captures the success signal. Code Generation builds the smallest
thing that tests it. Everything else is SKIP; launch and curation belong to
the production intent that follows.

## Membership

Analyze: intent-capture, requirements-analysis, feasibility-assessment.
Develop: code-generation.

Everything else is SKIP.
