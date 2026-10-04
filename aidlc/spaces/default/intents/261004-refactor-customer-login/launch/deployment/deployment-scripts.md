# Deployment scripts

No new scripts. `scripts/smoke.mjs` and `scripts/seed-customers.mjs` now use the proxy endpoints. Local stack: `make up` rebuilds identity-service, and migration 0007 is applied on start.
