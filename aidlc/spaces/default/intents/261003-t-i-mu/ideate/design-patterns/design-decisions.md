# Design Decisions

Greenfield repository — no existing codebase patterns to align with; all
choices are new and recorded as ADRs in `docs/adr/`.

| Problem | Forces | Decision (pattern) | Alternatives | Consequences | Status | ADR |
| --- | --- | --- | --- | --- | --- | --- |
| Repo organisation | shared contract, one team | Monorepo | polyrepo | atomic contract changes; CI path filters | Proposed | 0001 |
| First-party authentication | native UX, simplicity | Kratos sessions (opaque) | OAuth2/Hydra | Kratos round trip per validation | Proposed | 0002 |
| Client integration | CSRF safety, mobile UX | Browser flows (web) / API flows (mobile) | API flows in SPA, BFF, WebView | cookie-domain config | Proposed | 0003 |
| Population separation | no admin self-signup | Multiple identity schemas, non-selectable admin | 2 Kratos, metadata role | admin policy in service | Proposed | 0004 |
| Authorization | roles now, relationships later | Keto (Zanzibar ReBAC) behind `Authorizer` port | metadata RBAC, OPA | extra service | Proposed | 0005 |
| Authentication at API | latency, revocation | Middleware + cache-aside (TTL 30 s) | Oathkeeper, Redis | ≤ 30 s revocation lag | Proposed | 0006 |
| Persistence isolation | Ory migrations, least privilege | Database-per-component, DDL/DML roles | shared DB, clusters | per-role conn limits | Proposed | 0007 |
| Cross-system provisioning | webhook unreliability | Webhook + idempotent upsert (at-least-once + idempotent receiver) | interrupting webhook, polling | exactly one profile | Proposed | 0008 |
| Service structure | testability, swappable Ory | Hexagonal (ports & adapters), manual DI, repository pattern, sqlc | layered MVC, ORM | more boilerplate | Proposed | 0009 |
| Interface definition | 3 client languages | Contract-first OpenAPI, RFC 9457 | code-first, gRPC | YAML reviewed first | Proposed | 0010 |
| Resilience to Ory | fail closed | Timeouts + limited retries (reads only) + circuit breaker | none | 503 under outage | Proposed | docs/principles P10 |
| Idempotent creates | client retries | Idempotency-Key store (24 h) | none | extra table | Proposed | docs/api |

Reversibility: every row sits behind a port or config switch (e.g. Hydra,
Oathkeeper, second Kratos can be added without client rewrites) or is
justified in its ADR.
