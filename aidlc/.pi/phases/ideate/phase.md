---
slug: ideate
name: Ideate
order: 2
focus: Design
ai_role: Architect partner
output: Diagrams, specs
description: AI-driven architecture design and documentation generation
key_activities:
  - Architecture diagrams
  - Design pattern selection
  - API contract design
  - Technical specifications
example_prompts:
  - "Design an architecture for..."
  - "Generate a Mermaid diagram..."
  - "What patterns fit this..."
  - "Write a technical spec for..."
---

# Phase 2 — Ideate

Ideate turns approved requirements into an architecture and a set of design
decisions that the team can build against. The goal is not a perfect design;
it is a *reviewed, reversible* design with explicit trade-offs, clear
boundaries, and enough detail that implementation is mostly mechanical.

## Focus

Design.

## The AI's role

Architect partner. The model proposes options, drafts diagrams and contracts,
and enumerates trade-offs. The human chooses; the model must never silently
pick an architecture.

## Outputs

- Architecture document with component and deployment diagrams
- Recorded design-pattern and technology decisions
- API and interface contracts
- A technical specification that maps requirements to components

## Gates

Every stage in Ideate ends at a human approval gate. Architecture decisions
that are expensive to reverse require an explicit human choice, not a default.

## Exit criteria

- Each significant requirement maps to at least one component
- Contracts are explicit (inputs, outputs, errors, compatibility)
- Trade-offs and rejected options are recorded
- Non-functional targets (performance, security, cost) are stated

## Next phase

When Ideate is approved, [Develop](../develop/phase.md) decomposes the spec
into units of work and implements them.
