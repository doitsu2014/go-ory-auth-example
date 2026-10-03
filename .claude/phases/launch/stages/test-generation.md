---
slug: test-generation
name: Test Generation
phase: launch
execution: ALWAYS
condition: Always executes — generates the approved test suite
lead_agent: qa-agent
support_agents:
  - developer-agent
mode: inline
produces:
  - test-suite
consumes:
  - artifact: test-plan
    required: true
  - artifact: source-changes
    required: true
requires_stage:
  - test-plan
inputs: Test plan, code under test
outputs: test files, test-suite.md
---

# Test Generation

## Steps

### Step 1: Generate tests per the plan

Write unit tests for the happy path and the failure paths named in the plan.
Name tests after the requirement they verify.

### Step 2: Prefer real behaviour

Avoid over-mocking. Test the contract, not the implementation.

### Step 3: Run and stabilise

Run the suite. Eliminate flakiness (time, randomness, ordering, network).

### Step 4: Record traceability

Write `test-suite.md`: requirement id → test name → result.

### Step 5: Complete and confirm

Report `awaiting-approval` and present the results and coverage.
