# Design Principles

These rules apply to every change. A PR that breaks one must say which and why
in its description, and record an ADR if the deviation is lasting.

## P1. Don't own credentials

Ory Kratos owns passwords, MFA secrets, recovery and sessions. Our code never
receives, stores, logs or hashes a password. Clients send credentials **only**
to Kratos public.

*Test:* `grep -ri password services/` finds no handler that accepts one.

## P2. Use the flow made for the client

Browsers use Kratos browser flows (cookie + CSRF). Native apps use API flows
(session token). Never mix them; never wrap Kratos flows in our own API.

## P3. One source of truth per datum

Traits (email, name) live in Kratos, roles in Keto, app data in the identity
DB. Never duplicate across stores; join by **Kratos identity id** at read time.
If a copy is ever needed (e.g. search), it is a cache with a documented
refresh path.

## P4. Authenticate at every entry, authorize per use case, deny by default

Each request is authenticated by middleware. Each use case checks its own
permission. A route without a declared policy must not start.

## P5. The admin plane is a separate, stricter world

Separate route prefix (`/admin/v1`), separate schema (`admin`), mandatory MFA
(AAL2), shorter session age, invite-only, every mutation audited.

## P6. Private by default

Kratos admin, Keto, webhooks, metrics and the database are never internet-
reachable. Exposure is an explicit, reviewed decision.

## P7. Contract first

The OpenAPI document is written and reviewed before handlers. Server stubs and
client SDKs (TypeScript, Dart) are generated from it. Breaking changes need a
new major version.

## P8. Domain at the centre (hexagonal)

Business rules do not import HTTP, SQL or Ory SDK packages. They depend on
ports (interfaces); adapters implement them. This keeps Ory replaceable
(self-hosted ↔ Ory Network) and the domain testable without containers.

## P9. Idempotent integrations

Anything triggered by another system (webhooks, retries, client re-submits) is
safe to run twice: `ON CONFLICT DO NOTHING`, idempotency keys for create
operations, natural keys over generated ones.

## P10. Fail closed, degrade visibly

If Kratos or Keto is unreachable, protected endpoints return `503`, never
"allow". Timeouts on every outbound call. `/readyz` reflects dependencies.

## P11. Configuration is environment, secrets are never code

12-factor config. Secrets come from env/secret manager, are never committed,
never logged, and are rotatable without a code change.

## P12. Observable by default

Every request has a request id, structured logs, a trace, and RED metrics.
Logs carry identity ids, never PII or credentials.

## P13. Thin walking skeleton first

Build the thinnest end-to-end slice (compose stack → Kratos → API → one screen
in each client) before widening features. Each feature then cuts through all
layers.

## P14. Pin, reproduce, upgrade deliberately

Exact versions for images, Go modules, npm and pub packages. Ory upgrades are
their own PR with migration notes.

## P15. Decisions are recorded and reversible

Anything expensive to reverse gets an ADR with alternatives and consequences.
Prefer the option that keeps future options open.

## P16. Tests trace to requirements

Every FR/NFR has at least one test that names it. Happy-path floor per
component; security-relevant paths (authz denials) are always tested.

## P17. Simple over clever

Choose boring, well-documented tools. Add a component (Oathkeeper, Hydra, a
queue, a cache server) only when a requirement needs it, and record the trigger.
