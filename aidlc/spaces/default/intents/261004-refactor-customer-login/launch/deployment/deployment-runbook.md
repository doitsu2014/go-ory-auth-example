# Deployment runbook: customer login proxy (ADR-0014)

1. **Ship the app update first in the store.** It must be mandatory: older
   builds call `POST /v1/auth/identifiers`, which now answers 404, and talk
   to Kratos directly with resolved handles.
2. **Deploy identity-service.** Migration 0007 runs on start
   (`MIGRATE_ON_START`) or with `identity-service migrate up`. It is
   additive, backfills `lookup_key = pseudonym` and runs in one transaction.
   The table has one row per customer.
3. **Environment variables.**
   - Rename `LOGIN_RESOLVE_RATE` to `LOGIN_SIGNIN_RATE`.
   - Rename `LOGIN_RESOLVE_NET_RATE` to `LOGIN_NET_RATE`.
   - Add `LOGIN_ACCOUNT_RATE` (optional, default `10/15m,50/24h`).
   - The old names are ignored, so set the new ones if you customised them.
4. **Kratos.** No configuration change. Comments were updated in the schema
   and in `pre-registration.jsonnet` only.
5. **Verify.** Run `node scripts/smoke.mjs`; all checks should pass,
   including PLX. Then sign in an existing customer.
6. **Ingress hardening (follow-up).** Restrict Kratos
   `/self-service/{registration,recovery}/api` to identity-service and
   rate-limit `/self-service/login/api`.

**Rollback.** Redeploy the previous image. Migration 0007 down drops
`lookup_key`. Customers who registered after the deploy have random
handles, which the old resolve model cannot find: they would have to
recover through support. Roll back only before the app update spreads.
