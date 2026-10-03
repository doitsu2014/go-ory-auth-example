# Constraint Register — Encrypted personal information

| ID | Constraint | Source | Impact |
| --- | --- | --- | --- |
| PC-1 | Secrets never in source or logs | org memory | Tokens/unseal keys only via env/volume files; log redaction extended to PII fields |
| PC-2 | Least privilege | org memory | Separate app vs operator OpenBao policies; DB role `identity_app` gets DELETE only on `subject_key` |
| PC-3 | Kratos needs plaintext email | Kratos design | Kratos tables stay out of scope |
| PC-4 | Existing hexagonal layout, hand-written Ory adapters, sqlc + goose | develop phase of 261003-t-i-mu | New `crypto` domain port + `openbao` adapter in the same style |
| PC-5 | OpenAPI 3.0.3 contract is the source of truth | ADR-0010 | New endpoints added to `identity-service.v1.yaml` before code |
| PC-6 | Keto OPL `Console:main` | ADR | New permit `reveal_customer_pii` |
| PC-7 | Dependency changes are recorded | org memory | Only new image: `openbao/openbao:2.4.1` (MPL-2.0); no new Go modules expected |
