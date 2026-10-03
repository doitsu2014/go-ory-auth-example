---
slug: develop
name: Develop
order: 3
focus: Code
ai_role: Coding agent
output: Working code
description: AI coding agents and intelligent code generation
key_activities:
  - Delegate scoped tasks to agents
  - Review agent diffs
  - Refactoring assistance
  - Bug identification
example_prompts:
  - "Implement this function..."
  - "Review this code for..."
  - "Refactor to use..."
  - "Why is this failing..."
---

# Phase 3 — Develop

Develop decomposes the approved design into small, independently verifiable
units of work and implements them with coding agents. Authoring is cheap;
verification is the constraint. Every unit is produced behind a plan, a
review, and a human-approved diff.

## Focus

Code.

## The AI's role

Coding agent. The model plans, implements, and proposes diffs. The human
reads and understands every diff before it ships. Humans own merges.

## Outputs

- A unit breakdown with dependencies and acceptance criteria
- Implementation plans (reviewed before code is written)
- Working, tested code changes
- Review records for each unit

## Gates

- **Plan approval** before code generation begins
- **Diff review** before a unit is marked complete
- **Human merge**: an agent never merges its own work

## Exit criteria

- Each unit traces to a requirement and an acceptance criterion
- The project's own build, lint, and test commands pass
- Every diff was read by a human
- Deviations from the design are recorded

## Next phase

When Develop is approved, [Launch](../launch/phase.md) generates tests,
wires CI/CD, and validates the release.
