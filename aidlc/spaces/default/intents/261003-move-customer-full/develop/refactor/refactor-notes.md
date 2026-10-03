# Refactor Notes

No separate refactor pass. Review fixes already reshaped the migration
(`namemigration.go` rewritten around decide → store-with-ledger-recheck →
audited strip) and the erase path; no further structural debt was raised.

Carried forward (not now):
- `EraseMine` reads Kratos on every call to detect a legacy name trait. Remove
  that read once every environment reports `pii migrate-kratos-names` at 0.
- After all environments are migrated, `customer.v1.json` could get a new `$id`
  and the deprecated `Me.name` / `Customer.name` can be removed in API v2.
