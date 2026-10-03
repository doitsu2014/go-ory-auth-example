---
slug: ci-pipeline
name: CI Pipeline
phase: launch
execution: ALWAYS
condition: Always executes — automates build, test, and packaging
lead_agent: devops-agent
support_agents:
  - qa-agent
mode: inline
produces:
  - ci-config
consumes:
  - artifact: test-plan
    required: true
  - artifact: test-suite
    required: true
requires_stage:
  - test-generation
inputs: Test plan, test suite, project build commands
outputs: pipeline configuration files, ci-config.md
---

# CI Pipeline

## Steps

### Step 1: Discover the build contract

Find the exact commands for install, build, lint, and test. The pipeline must
use the same commands a developer runs.

### Step 2: Write the pipeline

Configure stages: checkout, setup, dependency cache, lint, build, test, and
package. Fail fast; run the cheapest gate first.

### Step 3: Make it deterministic

Pin dependency versions and tool versions. Avoid network at test time where
possible.

### Step 4: Document secrets and permissions

List required secrets and grant least privilege. Never echo secrets.

### Step 5: Complete and confirm

Report `awaiting-approval` and present the pipeline and a green run.
