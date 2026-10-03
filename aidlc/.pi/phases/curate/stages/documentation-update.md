---
slug: documentation-update
name: Documentation Update
phase: curate
execution: ALWAYS
condition: Always executes — documentation tracks the shipped system
lead_agent: tech-writer-agent
support_agents:
  - developer-agent
mode: inline
produces:
  - updated-docs
consumes:
  - artifact: source-changes
    required: false
  - artifact: technical-spec
    required: false
  - artifact: release-validation-report
    required: false
requires_stage:
  - release-validation
  - observability-setup
inputs: Code, spec, release report, existing docs
outputs: updated documentation, documentation-update.md
---

# Documentation Update

## Steps

### Step 1: Find stale docs

Compare the shipped system against existing docs: README, runbooks, API
references, and architecture notes.

### Step 2: Update in place

Correct rather than append. Remove instructions that no longer work.

### Step 3: Add the operational view

Document how to run, debug, and recover the system for someone on call.

### Step 4: Record what changed

Write `documentation-update.md`: files changed and the reason.

### Step 5: Complete and confirm

Report `awaiting-approval` and present the changes.
