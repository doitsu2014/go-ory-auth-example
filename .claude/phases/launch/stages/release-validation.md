---
slug: release-validation
name: Release Validation
phase: launch
execution: ALWAYS
condition: Always executes — validates the release against the requirements
lead_agent: qa-agent
support_agents:
  - product-agent
  - devops-agent
mode: inline
produces:
  - release-validation-report
consumes:
  - artifact: requirements
    required: true
  - artifact: test-suite
    required: true
  - artifact: deployment-runbook
    required: false
requires_stage:
  - test-generation
  - deployment
inputs: Requirements, test results, deployment runbook
outputs: release-validation-report.md
---

# Release Validation

## Steps

### Step 1: Re-run the full verification

Run the complete build, lint, and test suite on the release candidate. Record
the exact version or commit.

### Step 2: Validate requirement coverage

Confirm each must-have requirement has passing verification. List any
requirement not covered, with a decision and owner.

### Step 3: Validate operational readiness

Confirm rollback is possible, alerts are defined, and on-call knows what to
watch in the first hour.

### Step 4: State the release decision

Recommend go, go-with-known-issues (listed), or no-go — with reasoning.

### Step 5: Human release gate

A human explicitly approves the release. The model may recommend, never decide.

### Step 6: Complete and confirm

Report `awaiting-approval` and present the report and go/no-go gate.
