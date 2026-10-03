---
slug: design-patterns
name: Design Pattern Selection
phase: ideate
execution: ALWAYS
condition: Always executes — records the patterns and technologies chosen
lead_agent: design-agent
support_agents:
  - architect-agent
mode: inline
produces:
  - design-decisions
consumes:
  - artifact: architecture-doc
    required: true
  - artifact: requirements
    required: true
requires_stage:
  - architecture-design
inputs: Architecture document, requirements
outputs: design-decisions.md
---

# Design Pattern Selection

## Steps

### Step 1: Map forces to patterns

For each significant design problem (persistence, messaging, resilience,
state, concurrency), name the forces and the candidate patterns.

### Step 2: Record decisions

Use an ADR-style table: context, decision, alternatives, consequences, status.
Each record must be reversible-or-justified.

### Step 3: Align with the team

Prefer patterns already used in the codebase unless a deviation is justified.
Record deviations explicitly.

### Step 4: Complete and confirm

Report `awaiting-approval` and present the gate.
