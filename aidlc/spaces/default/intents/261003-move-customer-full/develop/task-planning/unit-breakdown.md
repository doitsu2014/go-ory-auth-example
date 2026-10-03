# Unit Breakdown

| Unit | Scope | Requirements | Depends on |
| --- | --- | --- | --- |
| N0 | OpenAPI contract (done by lead; Redocly 0 errors, 10 pre-existing warnings) | NAME-FR-02/04/05/07 | — |
| N1 | Go: migration 0005, sqlc, domain `pii.Name`, seal/open/mask/reveal/lookup, `/v1/me` + admin Customer stop returning name, Kratos `RemoveTraitName`, `pii migrate-kratos-names` CLI, Kratos customer schema, `dev`, `seed-customers.mjs`, `smoke.mjs` | NAME-FR-01..08, 10, NFR-01..04 | N0 |
| N2 | Admin web: types regen, masked name, reveal field, remove Kratos name row, tests, i18n | NAME-FR-09 | N0 |
| N3 | Mobile: sign-up without name, personal-info name fields, profile name, tests, l10n | NAME-FR-09 | N0 |
| N4 | Docs: 08, 04, 06, API guide, ADR-0011 amendment, README | NAME-NFR-05 | N1 |
