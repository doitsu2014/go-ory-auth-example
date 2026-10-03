---
slug: launch
name: Launch
order: 4
focus: Deploy
ai_role: QA engineer
output: Tests, pipelines
description: AI-generated tests and deployment automation
key_activities:
  - Test generation
  - CI/CD configuration
  - Deployment scripts
  - Release validation
example_prompts:
  - "Generate tests for..."
  - "Create a CI pipeline..."
  - "Write deployment script..."
  - "What should I test before..."
---

# Phase 4 — Launch

Launch makes the change releasable: it hardens verification with generated
tests, automates the pipeline, prepares deployment and rollback, and validates
the release against the requirements before it reaches production.

## Focus

Deploy.

## The AI's role

QA engineer. The model generates tests, pipeline configuration, and release
checklists. The human decides readiness and owns the go/no-go decision.

## Outputs

- A risk-based test plan and generated test suite
- CI pipeline configuration
- Deployment automation and a rollback runbook
- A release validation report tied to requirements

## Gates

- **Test plan approval** before the suite is generated
- **Release gate**: a human explicitly approves the release

## Exit criteria

- Requirements have corresponding tests (traceability)
- The pipeline builds, tests, and packages deterministically
- Deployment and rollback are documented and exercised
- A human has signed off on release readiness

## Next phase

When Launch is approved, [Curate](../curate/phase.md) observes production and
drives continuous improvement.
