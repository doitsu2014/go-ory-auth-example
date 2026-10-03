# Constraint Register — M2M

| ID | Constraint | Impact |
| --- | --- | --- |
| MC-1 | Credential bound to plane (ADR-0006, 06-security §6.4) | New `/m2m/v1` plane accepts only Hydra JWTs |
| MC-2 | Authorization declared per route; fail closed | Scope policy kind added to `policy.go` |
| MC-3 | Secrets never in source or logs | `SECRETS_SYSTEM` via `.env`; client secret shown once |
| MC-4 | Dependency changes recorded | `oryd/hydra:v26.2.0` image, `go-jose/v4` module |
| MC-5 | ADR-0002 "Revisit when … machine-to-machine clients appear" | ADR-0012 records the revisit; first-party clients keep Kratos sessions |
