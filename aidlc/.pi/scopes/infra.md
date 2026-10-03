---
name: infra
depth: Standard
keywords:
  - infra
  - infrastructure
  - terraform
  - kubernetes
  - pipeline
  - platform
  - observability
description: "Design, deploy, and operate infrastructure and platforms"
skeleton: off
review_cap: advisory
guard_policy: relaxed
sensors: on
learnings: on
summary_confirmation: off
phases:
  - analyze
  - ideate
  - launch
  - curate
skip:
  - research-synthesis
  - api-contract
  - code-review
  - refactor
---

# infra scope

`infra` is for platform and infrastructure work: environments, pipelines,
deployment automation, and observability. It replaces application code
generation with deployment and operations.

## Why these stages

Architecture Design and Technical Specification define the target topology.
The Launch tail builds CI, deploys, and validates. Curate installs
observability and handles performance and documentation. Application-focused
stages (API contract, refactor, code review) are skipped.

## Membership

Analyze: intent-capture, requirements-analysis, feasibility-assessment.
Ideate: architecture-design, design-patterns, technical-spec.
Develop: task-planning, code-generation.
Launch: test-plan, test-generation, ci-pipeline, deployment, release-validation.
Curate: all four stages.

Everything else is SKIP.
