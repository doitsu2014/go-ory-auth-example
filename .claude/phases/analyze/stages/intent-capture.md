---
slug: intent-capture
name: Intent Capture
phase: analyze
execution: ALWAYS
condition: Always executes — establishes what the work is and why it matters
lead_agent: product-agent
support_agents: []
mode: inline
produces:
  - intent-statement
  - intent-capture-questions
consumes: []
requires_stage: []
inputs: The user's request, existing docs, tickets, and domain knowledge
outputs: intent-statement.md, intent-capture-questions.md
---

# Intent Capture

## Steps

### Step 1: Load prior context

Read any user-supplied documents, tickets, or transcripts. Read project memory
(`aidlc/spaces/<space>/memory/project.md`) for standing goals and constraints.

### Step 2: Ask clarifying questions

Create `<record>/analyze/intent-capture/intent-capture-questions.md` and ask:

- What problem does this solve, and for whom?
- What does success look like, in observable terms?
- What is explicitly out of scope?
- Are there hard deadlines, budget, or regulatory constraints?
- What happens if we do nothing?

Follow the question flow in `protocols/question-flow.md`.

### Step 3: Analyse answers

Run ambiguity detection and contradiction analysis. Resolve open questions
within the stage; do not carry a contradiction into the next stage.

### Step 4: Generate the intent statement

Write a one-page statement: problem, users, success criteria, in/out of scope,
constraints, and assumptions.

### Step 5: Complete and confirm

Report the lifecycle outcome through the engine
(`my-aidlc orchestrate report --stage intent-capture --result awaiting-approval`)
and present the approval gate.
