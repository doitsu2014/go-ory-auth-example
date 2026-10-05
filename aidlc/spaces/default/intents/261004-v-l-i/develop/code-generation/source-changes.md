# Source Changes

Only docs changed. No code, config or OpenAPI changes (constraint K1).

## Added (`docs/features/`)

| File | Mermaid blocks |
| --- | --- |
| README.md | 2 (context, use-case) |
| F01-customer-registration.md | 3 |
| F02-customer-verification.md | 2 |
| F03-customer-login.md | 3 |
| F04-customer-recovery.md | 2 |
| F05-customer-settings-logout.md | 3 |
| F06-request-authentication.md | 3 |
| F07-customer-profile.md | 3 |
| F08-customer-personal-info.md | 3 |
| F09-admin-sign-in.md | 4 |
| F10-admin-management.md | 5 |
| F11-customer-management.md | 4 |
| F12-admin-pii-access.md | 3 |
| F13-audit-log.md | 2 |
| F14-service-clients.md | 3 |
| F15-machine-to-machine-api.md | 2 |
| F16-background-jobs.md | 4 |

Total: 51 new blocks.

## Modified

- `docs/README.md`: added a "Diagrams" note and reading-order row 16.
- `docs/architecture/01, 03, 06, 08, 09, 10`: added a "Current diagrams"
  pointer.
- `docs/architecture/03-auth-flows.md:134` and
  `10-pseudonymous-login.md:189`: replaced `;` with `,` inside the message
  text. Both blocks failed to parse before this change.
