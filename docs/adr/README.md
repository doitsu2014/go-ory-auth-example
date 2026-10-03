# Architecture Decision Records

Format: lightweight MADR — Context, Decision, Alternatives, Consequences, Status.
ADRs are immutable once **Accepted**; to change one, add a new ADR that
supersedes it.

| # | Title | Status |
| --- | --- | --- |
| [0001](0001-monorepo.md) | Monorepo for services, apps, infra and docs | Proposed |
| [0002](0002-kratos-sessions-over-oauth2.md) | Kratos sessions instead of OAuth2/OIDC (Hydra) for first-party apps | Proposed |
| [0003](0003-browser-flows-web-native-flows-mobile.md) | Browser flows for admin web, native API flows for mobile | Proposed |
| [0004](0004-single-kratos-two-identity-schemas.md) | One Kratos with `customer` and `admin` identity schemas | Proposed |
| [0005](0005-keto-for-authorization.md) | Ory Keto for authorization | Proposed |
| [0006](0006-session-validation-in-service.md) | Session validation in Go middleware with short cache (no Oathkeeper in v1) | Proposed |
| [0007](0007-database-per-component.md) | One PostgreSQL cluster, database + role per component | Proposed |
| [0008](0008-profile-provisioning.md) | Profile provisioning via webhook + idempotent lazy upsert | Proposed |
| [0009](0009-go-service-architecture.md) | Hexagonal Go service: chi, pgx + sqlc, goose, oapi-codegen | Proposed |
| [0010](0010-contract-first-api.md) | Contract-first OpenAPI 3.0.3 with RFC 9457 errors | Proposed |
| [0011](0011-envelope-encryption-for-pii.md) | Envelope encryption with OpenBao Transit for customer PII | Proposed |

Status flow: Proposed → Accepted (owner review) → Superseded / Deprecated.

## Template

```markdown
# NNNN. Title
- Status: Proposed | Accepted | Superseded by NNNN
- Date: YYYY-MM-DD

## Context
## Decision
## Alternatives considered
## Consequences
## Revisit when
```
