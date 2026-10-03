# Unit Breakdown — M2M with Hydra

| Unit | Scope | Requirements | Track | Depends on |
| --- | --- | --- | --- | --- |
| H1 | Compose: `hydra-db` provisioning, `hydra-migrate`, `hydra`, config, `.env.example`, Make targets, Keto `manage_service_clients` | M2M-FR-01, 14, NFR-01/02 | lead | — |
| H2 | Go: domain/machine, verifier (JWKS, claims, client-status cache), machine plane middleware + scope policies, `/m2m/v1` handlers | M2M-FR-02..07, NFR-03..05 | Go | H1 |
| H3 | Go: service-client management API + CLI, audit, idempotency/compensation | M2M-FR-08..11, 13 | Go | H1 |
| H4 | Admin web "Service clients" page | M2M-FR-12 | React | contract |
| H5 | Smoke checks, docs (09-machine-access, ADR-0012, ADR-0002 revisit note, 06-security, 05-deployment, API guide) | — | lead | H1–H4 |
