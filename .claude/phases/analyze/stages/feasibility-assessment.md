---
slug: feasibility-assessment
name: Feasibility Assessment
phase: analyze
execution: ALWAYS
condition: Always executes — tests whether the requirements can be met
lead_agent: architect-agent
support_agents:
  - product-agent
mode: inline
produces:
  - feasibility-assessment
  - constraint-register
consumes:
  - artifact: intent-statement
    required: true
  - artifact: requirements
    required: true
  - artifact: research-report
    required: false
requires_stage:
  - requirements-analysis
  - research-synthesis
inputs: Intent statement, requirements, research report, existing systems
outputs: feasibility-assessment.md, constraint-register.md
---

# Feasibility Assessment

## Steps

### Step 1: Assess technical feasibility

For each major requirement, judge feasibility against the current stack, team
skills, dependencies, and unknowns. Identify spikes needed.

### Step 2: Assess cost, schedule, and risk

Estimate ballpark effort, cost, and schedule. Record risks with likelihood,
impact, and mitigation in the constraint register.

### Step 3: Record hard constraints

Capture non-negotiables: compliance, latency, residency, licensing, backwards
compatibility, and budget ceilings.

### Step 4: Recommend a path

Recommend proceed, proceed-with-spikes, rescope, or stop — with reasoning.

### Step 5: Complete and confirm

Report `awaiting-approval` and present the gate.
