# Artifact Vocabulary

Artifacts are lowercase-kebab names. A stage's `produces` list names the
artifacts it writes; a stage's `consumes` list names the artifacts it reads.
The engine resolves each name to a path under the intent record directory.

## Registry

| Artifact | Produced by | Description |
| --- | --- | --- |
| `intent-statement` | intent-capture | Problem, users, success, scope, constraints |
| `intent-capture-questions` | intent-capture | Stage questions file |
| `requirements` | requirements-analysis | Prioritised, testable requirement set |
| `requirements-questions` | requirements-analysis | Stage questions file |
| `research-report` | research-synthesis | Evidence-backed findings |
| `competitive-analysis` | research-synthesis | Alternatives and positioning |
| `feasibility-assessment` | feasibility-assessment | Feasibility and recommendation |
| `constraint-register` | feasibility-assessment | Hard constraints and risks |
| `architecture-doc` | architecture-design | Architecture and diagrams |
| `architecture-questions` | architecture-design | Stage questions file |
| `design-decisions` | design-patterns | ADR-style decisions |
| `api-contract` | api-contract | Interface contracts |
| `technical-spec` | technical-spec | Buildable specification |
| `implementation-plan` | task-planning | Approach and commands |
| `unit-breakdown` | task-planning | Units of work and dependencies |
| `source-changes` | code-generation | Code changes in the workspace |
| `code-generation-notes` | code-generation | Commands run, deviations |
| `review-record` | code-review | Findings and verdict |
| `refactor-notes` | refactor | What changed and why |
| `test-plan` | test-plan | Risk-based test strategy |
| `test-suite` | test-generation | Generated tests and traceability |
| `ci-config` | ci-pipeline | Pipeline configuration |
| `deployment-runbook` | deployment | Deploy and rollback steps |
| `deployment-scripts` | deployment | Deployment automation |
| `release-validation-report` | release-validation | Release readiness |
| `observability-config` | observability-setup | Logs, metrics, alerts, SLOs |
| `incident-report` | incident-analysis | Timeline, cause, actions |
| `performance-report` | performance-optimization | Baseline, change, gain |
| `updated-docs` | documentation-update | Documentation changes |

## Rules

- Names are lowercase-kebab and match the producing stage's `produces` entry.
- Questions files end in `-questions`.
- A consumed artifact that is `required: true` and absent is a gap; the engine
  surfaces it in `consumes_absent`.
