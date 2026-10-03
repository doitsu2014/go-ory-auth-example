---
slug: code-review
name: Code Review
phase: develop
execution: ALWAYS
condition: Always executes — every agent-authored diff is reviewed
lead_agent: code-reviewer-agent
support_agents:
  - security-agent
mode: inline
produces:
  - review-record
consumes:
  - artifact: source-changes
    required: true
  - artifact: implementation-plan
    required: false
requires_stage:
  - code-generation
inputs: The diff, implementation plan, requirements
outputs: review-record.md
---

# Code Review

## Steps

### Step 1: Read the diff in full

Read every changed file. Do not skim. The bar for AI-authored code is the bar
for human code, applied with more suspicion.

### Step 2: Check against the contract

Verify the change satisfies its acceptance criteria and traces to a
requirement. Flag scope creep.

### Step 3: Check correctness and safety

Look for logic errors, unhandled errors, injection and authz issues, resource
leaks, and missing tests.

### Step 4: Classify findings

Each finding: severity (blocking, major, minor, nit), file and line, evidence,
and suggested fix. Blocking findings must be fixed before completion.

### Step 5: Record and complete

Write `review-record.md` with findings and a verdict. Report
`awaiting-approval` and present findings at the gate.
