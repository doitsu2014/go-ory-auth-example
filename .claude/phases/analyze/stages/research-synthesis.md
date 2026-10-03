---
slug: research-synthesis
name: Research Synthesis
phase: analyze
execution: CONDITIONAL
condition: Runs when the solution space is unfamiliar or build-vs-buy is open
lead_agent: research-agent
support_agents:
  - architect-agent
mode: inline
produces:
  - research-report
  - competitive-analysis
consumes:
  - artifact: intent-statement
    required: true
  - artifact: requirements
    required: false
requires_stage:
  - intent-capture
inputs: Intent statement, requirements, market and technical questions
outputs: research-report.md, competitive-analysis.md
---

# Research Synthesis

## Steps

### Step 1: Frame the research questions

Derive the questions that actually change a decision: build vs buy, library
selection, prior art, competitive expectations, and known failure modes.

### Step 2: Gather sources

Use web search, project docs, and code search. Record a citation for every
non-obvious claim. Mark anything unverified as an assumption.

### Step 3: Synthesise

Write `research-report.md` with findings, evidence, and implications. Write
`competitive-analysis.md` only when competitors or alternatives exist.

### Step 4: Feed decisions forward

List the decisions this research unblocks and hand them to Ideate.

### Step 5: Complete and confirm

Report `awaiting-approval` and present the gate.
