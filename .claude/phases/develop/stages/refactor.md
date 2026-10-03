---
slug: refactor
name: Refactoring
phase: develop
execution: CONDITIONAL
condition: Runs when review or build reveals structural debt worth addressing now
lead_agent: developer-agent
support_agents:
  - code-reviewer-agent
mode: inline
produces:
  - refactor-notes
consumes:
  - artifact: review-record
    required: true
  - artifact: source-changes
    required: true
requires_stage:
  - code-generation
  - code-review
inputs: Review record, current code, test suite
outputs: refactor-notes.md, refactored code
---

# Refactoring

## Steps

### Step 1: Justify the refactor

State the debt, its cost, and why now rather than later. Refactors without a
named benefit are rejected.

### Step 2: Establish a safety net

Confirm tests cover the code to be changed. If not, add characterisation tests
first.

### Step 3: Refactor in small steps

Keep behaviour identical. Make one mechanical change at a time and run tests
after each.

### Step 4: Record the change

Write `refactor-notes.md`: what moved, what changed shape, and what stayed the
same.

### Step 5: Complete and confirm

Report `awaiting-approval` and present the gate.
