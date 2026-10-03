---
slug: observability-setup
name: Observability Setup
phase: curate
execution: ALWAYS
condition: Always executes — the system must be observable against its targets
lead_agent: sre-agent
support_agents:
  - devops-agent
mode: inline
produces:
  - observability-config
consumes:
  - artifact: technical-spec
    required: false
  - artifact: release-validation-report
    required: true
requires_stage:
  - release-validation
inputs: Technical spec, release validation report
outputs: observability configuration, observability-config.md
---

# Observability Setup

## Steps

### Step 1: Derive signals from targets

For each non-functional requirement, define the metric, its target, and its
alert threshold.

### Step 2: Define logs, metrics, traces

Specify structured log fields, key metrics, and trace propagation. Prefer
fewer, higher-signal alerts over noisy ones.

### Step 3: Define dashboards and alerts

Provide the dashboard definition and alert routing (who is paged, when).

### Step 4: Define SLOs

State service level objectives and the error budget policy.

### Step 5: Complete and confirm

Report `awaiting-approval` and present the configuration and alert plan.
