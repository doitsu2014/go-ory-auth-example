# BEGIN my-aidlc:claude
# my-aidlc for Claude Code

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

## Commands

| Command | Purpose |
| --- | --- |
| `/aidlc` | Start or resume the active workflow |
| `/aidlc --status` | Show the active intent, scope, and stage |
| `/aidlc --doctor` | Validate the workspace and configuration |
| `/aidlc --version` | Print the framework version |

On the command line the same operations are available as
`my-aidlc status`, `my-aidlc doctor`, and `my-aidlc orchestrate next`.

## AI-DLC method (imported)

The method — layered practice files `org.md`, `team.md`, `project.md` — is
authored once at `aidlc/spaces/default/memory/` and pulled into Claude's
ambient context by `.claude/rules/aidlc.md`. Edit the method there, never in
the stub.

## What was installed

- `.claude/skills/aidlc/` — the orchestrator skill
- `.claude/rules/aidlc.md` — the ambient method import
- `.claude/agents/` — the 15 agent personas
- `.claude/phases/` — the five phases and their stages
- `.claude/scopes/` — the workflow profiles
- `.claude/protocols/` — stage, question, recovery, and learnings protocols
- `.claude/tools/` — the deterministic engine

## Personal overrides

Copy `.claude/settings.local.json.example` to `.claude/settings.local.json`
(gitignored) to override settings without affecting shared configuration.
# END my-aidlc:claude
