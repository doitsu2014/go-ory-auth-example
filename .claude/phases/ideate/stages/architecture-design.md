---
slug: architecture-design
name: Architecture Design
phase: ideate
execution: ALWAYS
condition: Always executes — turns requirements into a reviewable architecture
lead_agent: architect-agent
support_agents:
  - design-agent
mode: inline
reviewer: security-agent
review_class: advisory
produces:
  - architecture-doc
  - architecture-questions
consumes:
  - artifact: requirements
    required: true
  - artifact: feasibility-assessment
    required: true
  - artifact: constraint-register
    required: false
requires_stage:
  - feasibility-assessment
inputs: Requirements, feasibility assessment, constraint register
outputs: architecture-doc.md, architecture-questions.md
---

# Architecture Design

## Steps

### Step 1: Load prior context

Read requirements, the feasibility assessment, and the constraint register.

### Step 2: Ask architecture questions

Create `<record>/ideate/architecture-design/architecture-questions.md`
covering scale, consistency, availability, latency, deployment topology,
data ownership, and integration boundaries.

### Step 3: Propose options

Present two to four candidate architectures with explicit trade-offs. Never
silently choose. Record rejected options and why.

### Step 4: Document the chosen architecture

Write `architecture-doc.md` with a context diagram, component diagram, and
deployment diagram (Mermaid), plus responsibilities, data flow, and failure
modes.

### Step 5: Security review

Dispatch the `security-agent` as reviewer against the architecture document.
Address blocking findings; record accepted risks.

### Step 6: Complete and confirm

Report `awaiting-approval` and present the gate.
