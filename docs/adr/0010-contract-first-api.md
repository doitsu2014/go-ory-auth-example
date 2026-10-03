# 0010. Contract-first OpenAPI 3.0.3 with RFC 9457 errors

- Status: Proposed
- Date: 2026-10-03

## Context

Three codebases (Go, TypeScript, Dart) consume one API. Hand-written clients
drift; mobile apps can't be force-upgraded.

## Decision

- `api/openapi/identity-service.v1.yaml` is written and reviewed first.
- Generate: Go server interfaces (`oapi-codegen`), TS types/client
  (`openapi-typescript` + `openapi-fetch`), Dart client (`openapi-generator dart-dio`).
- Errors use RFC 9457 Problem Details with a stable `code`.
- CI runs `oasdiff breaking` against `main`; breaking changes require `/v2`.

## Alternatives considered

- **Code-first (annotations → spec)** — spec becomes an afterthought.
- **gRPC / Connect** — great typing, but browser + Flutter tooling and Kratos
  interplay are simpler with REST/JSON.
- **GraphQL** — flexible reads, overkill for this API size.

## Consequences

- API review happens on the YAML diff.
- Generated clients make breaking changes visible at compile time.

## Revisit when

High-volume internal service-to-service traffic appears → consider gRPC internally.
