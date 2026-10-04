# Deployment Scripts — Pseudonymous customer login identifiers

The repo deploys with Docker Compose locally. Production manifests are out of
scope (01-overview §1.1 non-goal); the steps below map one to one onto
Jobs/Deployments there. Every script is idempotent and reads secrets only
from the environment / `.env` (never hardcoded; `.env.example` holds
local-only values).

| Script / target | Purpose | Idempotent | Parameters |
| --- | --- | --- | --- |
| `make env` / `./dev up` (`ensure_env`) | Create `.env` or append keys added later (`KRATOS_COURIER_API_KEY`, `COURIER_DEDUPE_SECRET`, `LOGIN_MIGRATION_PHASE`) | yes (only missing keys; newline-safe) | — |
| `make up` / `./dev up` | Build and start the stack: identity DB migration `0006` (job), OpenBao init (new keys + policies, `deletion_allowed=false`), Kratos config rendered with both keys, identity-service | yes | `.env` |
| `make login-migrate` / `./dev migrate-logins [--dry-run]` | Legacy email traits → pseudonyms (vault + two-patch). Prints counts only | yes (2nd run `migrated=0`) | `--dry-run` |
| `make login-purge` | Unbound/orphan vault rows (≥ 24 h; daily schedule) | yes | `--older-than`, `--dry-run` via direct CLI |
| `make kratos-scrub` | Courier message retention + legacy-message scrub + expired-flow cleanup | yes | `KEEP_DAYS` (7), `PHASE` (`transition`\|`complete`), `KEEP_LAST` (24h) |
| `make kek-rotate` + `make keys-rewrap` | Rotate PII KEK **and** login KEK; re-wrap DEKs and the login vault | yes | operator token (short-lived) |
| `make reset` | Teardown incl. volumes (local only) | yes | — |
| Kratos schema switch | `deploy/ory/kratos/kratos.yml.tmpl`: `customer.v2.transition.json` → `customer.v2.json` + `LOGIN_MIGRATION_PHASE=complete` | config change | — |

Production equivalents:

- **Database:** migration `0006` runs as a Job with the migrator role.
- **OpenBao:** the two keys and the policy lines go through the
  infrastructure-as-code of the managed OpenBao/Vault.
- **Kratos:** the config is rendered from the secret manager, including the
  courier key (05-deployment §5.3).
- **identity-service env:** `KRATOS_COURIER_API_KEY`, `COURIER_DEDUPE_SECRET`,
  `LOGIN_MIGRATION_PHASE`, `SMS_PROVIDER=http` with
  `SMS_HTTP_URL`/`SMS_HTTP_TOKEN`, and the budgets.
- **CronJobs:** `pii purge-unbound-logins` (daily), `kratos-scrub` SQL and
  `kratos cleanup sql` (daily).
- **Alerts:** `deploy/observability/login-alerts.yml`.
