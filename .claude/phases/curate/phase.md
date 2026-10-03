---
slug: curate
name: Curate
order: 5
focus: Maintain
ai_role: SRE assistant
output: Fixes, updates
description: AI-powered monitoring and continuous improvement
key_activities:
  - Log analysis
  - Root cause analysis
  - Performance optimization
  - Documentation updates
example_prompts:
  - "Analyze these logs..."
  - "What caused this error..."
  - "How can I optimize..."
  - "Update docs to reflect..."
---

# Phase 5 — Curate

Curate keeps the system healthy and the knowledge current after launch. It
covers observability, incident analysis, performance work, and documentation
that tracks the code instead of rotting behind it.

## Focus

Maintain.

## The AI's role

SRE assistant. The model analyzes logs and metrics, proposes root causes and
fixes, and keeps documentation in sync. The human owns operational decisions.

## Outputs

- Observability configuration (logs, metrics, traces, alerts)
- Incident and root-cause analyses
- Performance findings and optimizations
- Documentation updates that reflect the shipped system

## Gates

Conditional stages (incident analysis, performance optimization) self-skip
when they do not apply, with a recorded reason. Every executed stage ends at
a human approval gate.

## Exit criteria

- The system is observable against its non-functional targets
- Incidents have recorded root causes and follow-up actions
- Documentation matches the current system
- Learnings are recorded in project memory

## Continuous loop

Curate feeds back into Analyze: new findings become new intents, and the
cycle repeats. The compounding asset is your context, not the model.
