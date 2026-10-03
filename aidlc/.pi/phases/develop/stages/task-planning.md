---
slug: task-planning
name: Task Planning
phase: develop
execution: ALWAYS
condition: Always executes — decomposes the spec into units of work
lead_agent: delivery-agent
support_agents:
  - developer-agent
mode: inline
produces:
  - implementation-plan
  - unit-breakdown
consumes:
  - artifact: technical-spec
    required: false
  - artifact: requirements
    required: true
requires_stage:
  - feasibility-assessment
inputs: Technical spec, requirements, existing codebase
outputs: implementation-plan.md, unit-breakdown.md
---

# Task Planning

## Steps

### Step 1: Load the spec and the codebase

Read the technical spec (when present) and scan the existing project for
structure, conventions, and test setup.

### Step 2: Decompose into units

Create `unit-breakdown.md`. Each unit is a vertical slice with: id, title,
requirement ids, acceptance criteria, files likely touched, dependencies, and
estimated size (aim for one review-sized change).

### Step 3: Sequence and expose risk

Order units by dependency and risk. Identify the walking skeleton — the
thinnest end-to-end slice that proves the architecture.

### Step 4: Write the implementation plan

Create `implementation-plan.md`: approach, conventions, commands for build,
lint, and test, and the review strategy.

### Step 5: Plan approval

Present the plan for explicit human approval before any code is generated.

### Step 6: Complete and confirm

Report `awaiting-approval` and present the gate.
