# Requirements — Questions

Mode yolo; recommendations recorded.

| # | Question | Recommendation |
| --- | --- | --- |
| 1 | Where does the doc go? | New chapter `docs/architecture/10-pseudonymous-login.md` (chapters 01–09 exist) |
| 2 | Diagram notation? | Mermaid `sequenceDiagram` with coloured `rect` blocks per phase, mirroring the numbered sections of the owner's diagram |
| 3 | Source of truth for steps? | The code: `internal/app/login.go`, `courier.go`, `httpapi/router.go`, jsonnet templates, `kratos.yml.tmpl`, OpenBao policy names |
| 4 | Accuracy check? | Each diagram step cites the real endpoint/key; Mermaid syntax validated with the mermaid CLI if available, otherwise by review |
