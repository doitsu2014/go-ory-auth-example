# Deployment Runbook — Pseudonymous customer login identifiers

## 1. Environments

| Env | Differences |
| --- | --- |
| local (compose) | `APP_ENV=local`, OpenBao `http` with a local unseal key on a volume, `SMS_PROVIDER=sink` (Mailpit `<digits>@sms.local`), `LOGIN_REGISTER_RATE=60/1m,1000/24h` (seeding/smoke) |
| staging | As production with a test SMS provider account and a staging OpenBao. Run the full migration rehearsal on a copy of production Kratos data |
| production | `APP_ENV=production` (https OpenBao, auto-unseal), `SMS_PROVIDER=http`, default limits, `TRUSTED_PROXY_HOPS` set to the ingress depth, alerts loaded, ≥ 2 identity-service replicas |

## 2. Pre-conditions

1. The mobile build that resolves identifiers first is released, and a
   **minimum app version** is enforced. Older builds can no longer register
   (4049001) and cannot sign in after migration.
2. The admin web from this release is deployed in the same window. The list
   no longer accepts `?email=`.
3. OpenBao keys `identity-login-pseudonym` and `identity-login-kek` exist
   with `deletion_allowed=false`. An OpenBao snapshot has been taken and the
   **restore drill done** (losing the pseudonym key locks every customer
   out).
4. A Kratos DB and identity DB backup has been taken.
5. The courier key and the dedupe secret are created in the secret manager.
   The courier key is distinct from the webhook key; the service refuses to
   start otherwise.
6. CI is green on the release commit (`.github/workflows/ci.yml`).

## 3. Deploy (phases, ADR-0013 §8.12)

| Step | Action | Verify |
| --- | --- | --- |
| 1 | identity-service with migration `0006`, `LOGIN_MIGRATION_PHASE=transition` | `/readyz` 200; `identity_http_requests_total` normal; `POST /v1/auth/identifiers` 200 |
| 2 | Kratos config: transition schema, `profile` method off, pre-registration hook, `courier.delivery_strategy: http` | Kratos ready; registering a test customer through the app → code arrives; `identity_courier_dispatch_total{outcome="sent"}` rises; `identity_login_preregistration_total{result="allowed"}` |
| 3 | `pii migrate-kratos-logins --dry-run`, then the real run; repeat until `failed=0` | Counters; a second run gives `migrated=0`; a sample of legacy customers can sign in |
| 4 | Final schema `customer.v2.json` + `LOGIN_MIGRATION_PHASE=complete` (restart identity-service) | A legacy-shaped registration is rejected by the schema; admin login and mail still work |
| 5 | ≥ 24 h later: `make kratos-scrub PHASE=complete` (or `KEEP_LAST=1h` once every flow has expired) | SQL probe: no customer address in the Kratos tables (smoke `PLI-NFR-01` query) |
| 6 | Schedule purge, scrub and cleanup jobs; load the alerts | First purge run reports counts; alerts visible |

**Signal of a successful deployment:**

- `/readyz` is green.
- Smoke-equivalent checks pass in the target environment: email and phone
  registration, verification and sign-in, and the SQL probe returns 0 rows.
- Migration reports `failed=0`.
- `identity_courier_dispatch_total{outcome="error"}` stays at 0 for 30 min.

## 4. Health and readiness

| Check | Meaning |
| --- | --- |
| identity-service `/readyz` | DB, Kratos public/admin, Keto. OpenBao is deliberately not part of readiness: PII and login endpoints fail closed instead |
| `identity_courier_dispatch_total{outcome}` | sent / duplicate / dropped{reason} / error |
| `identity_login_preregistration_total{result}` | allowed / unresolved / legacy_traits / duplicate / error |
| `identity_login_unbound_rows` | Abuse or registration drop-off indicator |
| Kratos courier queue | `courier_messages` status `queued` count should stay small (retries pending) |

## 5. Rollback

| From step | Rollback | Data effect | Exercised? |
| --- | --- | --- | --- |
| 1 (service only) | Previous identity-service image. Migration `0006` stays; the old code ignores the new tables | none | **Not exercised.** Low risk (additive tables) |
| 2 (Kratos config) | Previous Kratos config: SMTP courier, `customer.v1.json`, profile method on. **Precondition:** no customer registered with `login_id` yet; otherwise those identities fail v1 schema validation | none if no new registrations | **Not exercised** |
| 3–5 (after migration) | **Roll forward only.** Kratos holds pseudonyms; the old service and old apps cannot read them. Emergency reverse: for each vault row, decrypt and two-patch `login_id` → `email` (inverse of the migration). No CLI exists (follow-up); only by a reviewed script | Re-introduces plaintext into Kratos | **Not exercised, untested** |
| Local env | `make reset && ./dev up` from the previous commit (fresh data) | local data lost | yes (standard dev flow) |

Because steps 3–5 are one-way, the go/no-go gate is **after step 2**:

- run 24 h on the transition config with new registrations healthy;
- then migrate.

## 6. Local execution log (this intent)

- **2026-10-04:** `./dev up` from this worktree replaced the running local
  stack, which had been started from another checkout. The owner chose this
  and the stack's `.env` was reused.
  - Migration: `scanned=32 migrated=32 failed=0`; second run `migrated=0`.
  - Seed: phone customer created through SMS sink verification.
  - Smoke: 47/47.
  - Go integration and e2e: green (twice).
  - Flutter integration: 7/7.
- **Local stack state:** this branch, still on the **transition** config
  (`LOGIN_MIGRATION_PHASE=transition`); step 4 (final schema) was not applied
  locally.
- **Effect on the other checkout:** its identity-service build expects
  `traits.email`, so it is not compatible with the migrated customers. Run
  the stack from this branch (or merge it) from now on.
