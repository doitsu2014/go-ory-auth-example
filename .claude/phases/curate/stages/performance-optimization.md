---
slug: performance-optimization
name: Performance Optimization
phase: curate
execution: CONDITIONAL
condition: Runs when metrics show a budget miss or a named performance goal exists
lead_agent: performance-agent
support_agents:
  - developer-agent
mode: inline
produces:
  - performance-report
consumes:
  - artifact: observability-config
    required: true
  - artifact: technical-spec
    required: false
requires_stage:
  - observability-setup
inputs: Metrics, profiles, traces, architecture
outputs: performance-report.md
---

# Performance Optimization

## Steps

### Step 1: Establish the baseline

Measure before changing anything. Record the exact workload and environment.

### Step 2: Locate the bottleneck

Use profiles and traces. Optimise the measured bottleneck, never the guessed
one.

### Step 3: Propose changes

For each change: expected gain, risk, and cost. Prefer the smallest change
with the largest measured effect.

### Step 4: Verify the gain

Re-measure under the same workload. Report the actual delta and any regression.

### Step 5: Complete and confirm

Report `awaiting-approval` and present the report.
