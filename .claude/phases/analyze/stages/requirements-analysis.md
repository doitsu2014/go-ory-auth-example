---
slug: requirements-analysis
name: Requirements Analysis
phase: analyze
execution: ALWAYS
condition: Always executes — converts intent into testable requirements
lead_agent: business-analyst-agent
support_agents:
  - product-agent
mode: inline
produces:
  - requirements
  - requirements-questions
consumes:
  - artifact: intent-statement
    required: true
requires_stage:
  - intent-capture
inputs: Intent statement, domain knowledge, existing documentation
outputs: requirements.md, requirements-questions.md
---

# Requirements Analysis

## Steps

### Step 1: Load prior context

Read the intent statement from the intent-capture record. Read any existing
requirement or specification documents.

### Step 2: Elicit and structure requirements

Create `<record>/analyze/requirements-analysis/requirements-questions.md`
covering functional gaps, non-functional targets, edge cases, data, and
integrations. Follow the question flow.

### Step 3: Classify and prioritise

For each requirement record: id, statement, type (functional, non-functional,
constraint, assumption), priority (MoSCoW), source, acceptance criteria, and
verification method.

### Step 4: Trace and validate

Ensure every requirement traces to the intent statement. Flag orphan
requirements. Reject any requirement that has no testable acceptance criteria.

### Step 5: Complete and confirm

Report `awaiting-approval` and present the gate.
