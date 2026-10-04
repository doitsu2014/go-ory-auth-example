# Test Plan — docs

| Risk | Check | Exit criterion |
| --- | --- | --- |
| Diagram does not render on GitHub | `mmdc` (mermaid-cli 11) on every block | 5/5 render |
| Diagram contradicts the code | Step-by-step review against `login.go`, `courier.go`, `router.go`, jsonnet, policies (done in code generation) | No mismatch |
| Broken links / anchors | Script: resolve every relative link and `#anchor` in the changed docs | 0 broken |

No unit/integration tests apply (no code change).
