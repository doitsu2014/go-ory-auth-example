---
slug: analyze
name: Analyze
order: 1
focus: Requirements
ai_role: Research assistant
output: Specs, analysis docs
description: AI-assisted requirements gathering and research synthesis
key_activities:
  - Requirements extraction
  - Research synthesis
  - Competitive analysis
  - Feasibility assessment
example_prompts:
  - "Summarize these requirements..."
  - "What are the risks of..."
  - "Compare approaches for..."
  - "What questions should I ask..."
---

# Phase 1 — Analyze

Analyze turns a raw request, idea, or ticket into a validated, traceable,
prioritized set of requirements plus the research and constraints that bound
them. It is the phase where the cost of a wrong assumption is lowest, so the
work is deliberately skeptical: every requirement must trace to a source, and
every claim that changes scope must be verified.

## Focus

Requirements.

## The AI's role

Research assistant. The model reads, summarizes, compares, and drafts. The
human owns validation: which requirements are real, which constraints are
hard, and what "done" means.

## Outputs

- An intent statement with explicit in/out boundaries
- A prioritized, testable requirement set
- A research and competitive synthesis with cited sources
- A feasibility assessment and constraint register

## Gates

Every stage in Analyze ends at a human approval gate. A stage is not complete
until the human has approved the artifact or requested changes.

## Exit criteria

- Every requirement is testable and traceable to a source
- Ambiguities and contradictions are resolved in-stage
- The scope boundary is explicit
- Hard constraints are recorded and visible downstream

## Next phase

When Analyze is approved, [Ideate](../ideate/phase.md) turns the requirement
set into an architecture, design decisions, and a technical specification.
