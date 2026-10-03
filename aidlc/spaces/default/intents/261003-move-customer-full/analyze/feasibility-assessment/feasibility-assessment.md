# Feasibility Assessment — Customer name as encrypted PII

| Area | Assessment |
| --- | --- |
| Crypto | Reuses the existing envelope scheme: one more column under the same DEK and AAD layout. No new key, no KMS change. Low risk. |
| Kratos | Removing a trait from the schema is supported; Kratos validates traits on writes, so identities that still carry `name` keep logging in but would fail a settings update until migrated. JSON Patch on `PATCH /admin/identities/{id}` (with `test`) lets the migration remove the trait atomically per identity. Medium risk: ordering between schema rollout and migration (mitigated: `./dev up` runs the migration right after start; production runbook in docs). |
| Data | Additive migration `0005` (`name_ct bytea NULL`, column grants). Existing rows untouched. |
| Clients | Mobile sign-up loses two fields; personal-info screen gains two. Admin web reads masked name from existing endpoints. Contract-first: OpenAPI change, regenerated types. |
| Effort | Go (domain, crypto columns, migration CLI, API) + web + mobile + docs: one developer pass per surface, then review. |

Verdict: **feasible**, no blockers.
