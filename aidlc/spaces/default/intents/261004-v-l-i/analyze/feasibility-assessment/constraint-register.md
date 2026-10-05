# Constraint Register

| ID | Constraint | Source | Impact |
| --- | --- | --- | --- |
| K1 | Docs only. No changes to code, config or OpenAPI. | Intent, out of scope | The diff is limited to `docs/` and `aidlc/` |
| K2 | Mermaid must render on GitHub | Repo convention (19 existing blocks) | No PlantUML or images |
| K3 | English text | Existing `docs/` language | — |
| K4 | No secrets or real PII in examples | org.md: "Secrets never appear in source or logs" | Use placeholders such as `handle`, `lookup_key`, `<client_secret>` |
| K5 | The code is authoritative over prose | Intent | When a doc and the code disagree, draw the code and flag the doc |
