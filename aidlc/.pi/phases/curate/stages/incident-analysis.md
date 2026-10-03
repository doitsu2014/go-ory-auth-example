---
slug: incident-analysis
name: Incident Analysis
phase: curate
execution: CONDITIONAL
condition: Runs when an incident or regression has occurred
lead_agent: sre-agent
support_agents:
  - developer-agent
mode: inline
produces:
  - incident-report
consumes:
  - artifact: observability-config
    required: false
  - artifact: source-changes
    required: false
requires_stage:
  - observability-setup
inputs: Logs, metrics, traces, timeline, code
outputs: incident-report.md
---

# Incident Analysis

## Steps

### Step 1: Build the timeline

Reconstruct what happened, when, and what was known at each point. Facts only.

### Step 2: Find the root cause

Use the logs and metrics to identify the causal chain, not just the trigger.
State confidence and competing hypotheses.

### Step 3: Assess impact

Quantify duration, affected users or systems, and data impact.

### Step 4: Define corrective actions

List fixes and preventions with owners and priorities. Distinguish the
immediate fix from the systemic prevention.

### Step 5: Complete and confirm

Report `awaiting-approval` and present the report.
