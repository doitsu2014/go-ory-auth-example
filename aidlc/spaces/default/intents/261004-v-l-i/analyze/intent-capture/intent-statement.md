# Intent Statement — Redraw functional and sequence diagrams per feature

Request (VI): "vẽ lại functional diagrams, sequences diagram cho từng tính
năng của hệ thống này". In English: redraw the functional diagrams and sequence
diagrams for every feature of the system.

## Problem

The system has a Go identity-service, Ory Kratos/Keto/Hydra, OpenBao, a Flutter
customer app, and a React admin console. Its diagrams are spread across
`docs/architecture/01…10`, and they are organised by concern (auth flows, PII,
machine access), not by feature. ADR-0013 (pseudonymous login identifiers) and
ADR-0014 (customer login through identity-service) changed the customer flows,
so some of the older diagrams no longer match the code.

## Users

| User | Need |
| --- | --- |
| Developers / reviewers | See exactly which component calls which, for one feature, on one page |
| Operators / security | See where secrets, PII and audit events show up in each flow |
| New joiners | A map from features to diagrams to code |

## Success criteria

1. `docs/features/README.md` has a feature catalogue: a system functional
   (use-case) diagram, a context diagram, and a table that links each feature
   page.
2. Each feature page has:
   - purpose and actors;
   - a **functional diagram**: a Mermaid flowchart of the decision and branch
     logic;
   - one or more **sequence diagrams**: Mermaid `sequenceDiagram`, with real
     HTTP paths, Keto relations, tables and audit events;
   - error branches;
   - code references (`file:line`).
3. Every step is checked against the current code on branch
   `doitsu2014/plankton`.
4. The docs index links to the catalogue, and stale diagrams in
   `docs/architecture/` are flagged.

## In scope

Every feature exposed by `api/openapi/identity-service.v1.yaml`, the Kratos
webhooks, the mobile and admin-web auth flows, M2M access, and the background
jobs.

## Out of scope

Code, config and OpenAPI changes, and any rewrite of the existing architecture
narrative.

## Constraints and assumptions

- Mermaid renders on GitHub, so no image build step is needed.
- The docs are written in English, matching the rest of `docs/`.
- If the code and a doc disagree, the code wins.
