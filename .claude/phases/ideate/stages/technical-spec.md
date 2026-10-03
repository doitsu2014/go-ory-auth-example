---
slug: technical-spec
name: Technical Specification
phase: ideate
execution: ALWAYS
condition: Always executes — the buildable spec the Develop phase consumes
lead_agent: design-agent
support_agents:
  - architect-agent
  - developer-agent
mode: inline
produces:
  - technical-spec
consumes:
  - artifact: architecture-doc
    required: true
  - artifact: design-decisions
    required: true
  - artifact: api-contract
    required: false
  - artifact: requirements
    required: true
requires_stage:
  - design-patterns
  - api-contract
inputs: Architecture, design decisions, API contracts, requirements
outputs: technical-spec.md
---

# Technical Specification

## Steps

### Step 1: Map requirements to components

Build a traceability table: requirement id → component(s) → interface(s) →
test approach. Every must-have requirement must appear.

### Step 2: Specify components

For each component: responsibility, inputs, outputs, state, dependencies,
error handling, and configuration.

### Step 3: Specify cross-cutting concerns

Security, observability, performance budgets, and data lifecycle.

### Step 4: Define the work breakdown seeds

Propose candidate units of work with rough dependency order. Develop refines
these; Ideate only seeds them.

### Step 5: Complete and confirm

Report `awaiting-approval` and present the gate.
