---
slug: code-generation
name: Code Generation
phase: develop
execution: ALWAYS
condition: Always executes — implements each approved unit
lead_agent: developer-agent
support_agents: []
mode: inline
for_each: unit-breakdown
workspace_requires: true
produces:
  - source-changes
  - code-generation-notes
consumes:
  - artifact: implementation-plan
    required: true
  - artifact: unit-breakdown
    required: true
  - artifact: technical-spec
    required: false
requires_stage:
  - task-planning
inputs: Implementation plan, unit breakdown, technical spec, codebase
outputs: code changes in the workspace, code-generation-notes.md
---

# Code Generation

Runs once per unit in `unit-breakdown.md`.

## Steps

### Step 1: Select the unit

Take the next ready unit. Confirm its acceptance criteria and dependencies.

### Step 2: Confirm plan approval

Do not write code until the plan for this unit is approved. If the plan
changed after approval, re-open plan approval unless the guard policy is
lowered by the active scope.

### Step 3: Implement

Follow the project's existing conventions. Keep the diff scoped to the unit.
Write or update tests alongside the change.

### Step 4: Verify locally

Run the project's build, lint, and test commands. Record the exact commands
and results in `code-generation-notes.md`.

### Step 5: Record deviations

Note any deviation from the technical spec, with reasoning. Unrecorded
deviation is a defect in process.

### Step 6: Complete and confirm

Report `awaiting-approval` and present the diff for human review.
