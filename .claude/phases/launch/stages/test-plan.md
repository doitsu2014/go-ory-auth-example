---
slug: test-plan
name: Test Plan
phase: launch
execution: ALWAYS
condition: Always executes — defines risk-based verification before tests are written
lead_agent: qa-agent
support_agents:
  - developer-agent
mode: inline
produces:
  - test-plan
consumes:
  - artifact: requirements
    required: true
  - artifact: technical-spec
    required: false
  - artifact: source-changes
    required: false
requires_stage:
  - code-review
inputs: Requirements, technical spec, code changes
outputs: test-plan.md
---

# Test Plan

## Steps

### Step 1: Map requirements to risks

Rank requirements by risk (impact × likelihood). The riskiest paths get the
deepest testing.

### Step 2: Define the strategy

State the test levels (unit, integration, end-to-end), the happy-path floor per
component, and what is deliberately not tested.

### Step 3: Define environments and data

List the environments, fixtures, and test data needed. No production data in
tests.

### Step 4: Define exit criteria

State measurable pass criteria, coverage floor, and known-acceptable gaps.

### Step 5: Plan approval

Present the test plan for human approval before generating the suite.

### Step 6: Complete and confirm

Report `awaiting-approval` and present the gate.
