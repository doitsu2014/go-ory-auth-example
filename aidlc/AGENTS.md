# BEGIN my-aidlc:pi
# my-aidlc for PI Agent

This project uses **my-aidlc**, an AI-Driven Development Life Cycle workflow
engine built around five phases: **Analyze → Ideate → Develop → Launch →
Curate**.

## Start a workflow

Type `/aidlc` followed by what you want to build:

```text
/aidlc Build a REST API for inventory management
```

The workflow profile is detected from the request. You can name one explicitly:
`classic`, `express`, `feature`, `bugfix`, `mvp`, `poc`, `infra`.

Force the skill when needed: `/skill:aidlc`.

## Commands

| Command | Purpose |
| --- | --- |
| `/aidlc` | Start or resume the active workflow |
| `/aidlc --status` | Show the active intent, scope, and stage |
| `/aidlc --doctor` | Validate the workspace and configuration |
| `/aidlc --version` | Print the framework version |

On the command line the same operations are available as
`my-aidlc status`, `my-aidlc doctor`, and `my-aidlc orchestrate next`.

## What was installed

- `.pi/skills/aidlc/` — the orchestrator skill (PI loads it on demand)
- `.pi/prompts/aidlc.md` — the `/aidlc` prompt template
- `.pi/agents/` — the 15 agent personas
- `.pi/phases/` — the five phases and their stages
- `.pi/scopes/` — the workflow profiles
- `.pi/protocols/` — stage, question, recovery, and learnings protocols
- `.pi/tools/` — the deterministic engine

## Workspace

`aidlc/` holds the workspace: memory (`spaces/default/memory/`), intent
artifacts (`spaces/default/intents/`), `state.json`, and `audit.log`.

## Principles

- Every requirement traces to a source and has a testable acceptance criterion.
- Authoring is cheap; verification is the constraint.
- Humans own merges. Every agent diff is read before it ships.
- The compounding asset is your context, not the model.
# END my-aidlc:pi
