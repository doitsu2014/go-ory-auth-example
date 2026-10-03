---
slug: api-contract
name: API Contract Design
phase: ideate
execution: ALWAYS
condition: Always executes when the system exposes or consumes an interface
lead_agent: architect-agent
support_agents:
  - developer-agent
mode: inline
produces:
  - api-contract
consumes:
  - artifact: architecture-doc
    required: true
  - artifact: requirements
    required: true
requires_stage:
  - architecture-design
inputs: Architecture document, requirements
outputs: api-contract.md
---

# API Contract Design

## Steps

### Step 1: Enumerate interfaces

List every interface to design: public APIs, internal service contracts,
events, CLIs, and data schemas.

### Step 2: Specify each contract

For each endpoint or message: method, path/name, request schema, response
schema, error model, idempotency, pagination, and auth.

### Step 3: Define compatibility

State the versioning strategy, deprecation policy, and what counts as a
breaking change.

### Step 4: Provide examples

Include at least one request/response example and one error example per
contract.

### Step 5: Complete and confirm

Report `awaiting-approval` and present the gate.
