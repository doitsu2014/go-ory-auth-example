# Architecture — Feature diagram set

## Information architecture

```
docs/features/
  README.md                       catalogue, use-case diagram, context diagram, legend
  F01-customer-registration.md
  F02-customer-verification.md
  F03-customer-login.md
  F04-customer-recovery.md
  F05-customer-settings-logout.md
  F06-request-authentication.md
  F07-customer-profile.md
  F08-customer-personal-info.md
  F09-admin-sign-in.md
  F10-admin-management.md
  F11-customer-management.md
  F12-admin-pii-access.md
  F13-audit-log.md
  F14-service-clients.md
  F15-machine-to-machine-api.md
  F16-background-jobs.md
```

`docs/README.md` links `features/README.md`. The architecture docs
(01, 03, 08, 09, 10) link to the matching feature pages.

## Page template

1. **Title and one-line purpose**
2. **Actors and entry points**: a table of actor, client, and endpoint(s)
3. **Functional diagram**: `flowchart TD` showing the decision logic
   (validation, authn, authz, branches, outcomes)
4. **Sequence diagram(s)**: `sequenceDiagram` with `autonumber`; the
   happy path plus `alt`/`opt` for the main failures
5. **Errors and outcomes**: a table of condition, HTTP status / problem type,
   and UI behaviour
6. **Side effects**: tables written, audit events, Keto tuples, emails/SMS
7. **Code references**: `path:line` list
8. **Related**: ADRs and architecture sections

## Canonical participants (same names on every page)

| Alias | Name | Notes |
| --- | --- | --- |
| `MA` | Mobile app (Flutter) | customer |
| `AW` | Admin web (React SPA) | browser, Kratos cookies |
| `IS` | identity-service | Go, :8080 |
| `KP` | Kratos public | :4433 |
| `KA` | Kratos admin | :4434 |
| `KR` | Keto read / `KW` Keto write | :4466 / :4467 |
| `HY` | Hydra | public :4444 / admin :4445 |
| `OB` | OpenBao | transit (HMAC, KEK wrap) |
| `DB` | identity DB (PostgreSQL) | profiles, pii, login vault, audit |
| `ML` | SMTP / SMS | Mailpit locally |
| `PS` | Partner service | M2M client |

## Conventions

- Never put `;` in message text, because Mermaid treats it as a statement
  break. Quote flowchart labels that contain `()`, `{}` or `/`.
- Show sensitive values by role (`handle`, `lookup_key`, `ciphertext`), never
  as realistic data.
- Mark the points where PII is encrypted or decrypted with `Note over IS`.
