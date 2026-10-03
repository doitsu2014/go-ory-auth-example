---
slug: deployment
name: Deployment Automation
phase: launch
execution: ALWAYS
condition: Always executes when the deliverable is deployed rather than shipped as a library
lead_agent: devops-agent
support_agents:
  - sre-agent
mode: inline
produces:
  - deployment-runbook
  - deployment-scripts
consumes:
  - artifact: ci-config
    required: true
  - artifact: technical-spec
    required: false
requires_stage:
  - ci-pipeline
inputs: CI config, architecture, environments
outputs: deployment scripts, deployment-runbook.md
---

# Deployment Automation

## Steps

### Step 1: Define environments

List environments (dev, staging, production) and their differences.

### Step 2: Write deployment scripts

Provide idempotent, repeatable apply and teardown. Parameterise environment
and version; never hardcode secrets.

### Step 3: Write the runbook

Document pre-conditions, the deploy steps, how to verify, and — critically —
how to roll back, with the rollback exercised or explicitly noted as untested.

### Step 4: Define health and readiness

State the health checks and the signal that means "deployed successfully".

### Step 5: Complete and confirm

Report `awaiting-approval` and present the scripts and runbook.
