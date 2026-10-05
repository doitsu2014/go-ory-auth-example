# Feasibility Assessment

## Verdict: Feasible (low risk)

| Dimension | Finding |
| --- | --- |
| Technical | Mermaid `flowchart` and `sequenceDiagram` cover every requirement. GitHub renders them natively. |
| Verification | `mermaid` 11 + `jsdom` in Node parses every block offline (set up in the scratchpad). It flags the same syntax errors GitHub would. |
| Source of truth | The code is traceable: OpenAPI has 40 operations, and the Kratos webhooks, the mobile/admin-web clients and the Go app layer are all readable. Four parallel code traces extract `file:line` facts for each feature. |
| Effort | 16 feature pages plus an index. These are docs only, with no build or deploy. |

## Evidence found during assessment

The existing diagrams already have defects:

- 19 Mermaid blocks exist in `docs/`, and **2 fail to parse**:
  - `docs/architecture/03-auth-flows.md:125`
  - `docs/architecture/10-pseudonymous-login.md:173`

  The cause is a `;` inside message text. Mermaid treats `;` as a statement
  separator, so GitHub shows a syntax-error box instead of the diagram. This
  confirms the need for the redraw and for an automated parse check.

## Risks

| Risk | Likelihood | Mitigation |
| --- | --- | --- |
| Diagram drifts from the code again | Medium | Each page cites `file:line`. The parse script is recorded in the test suite. |
| Very large sequence diagrams become unreadable | Medium | Split each feature into a happy-path sequence plus a separate error/branch flowchart, and use `autonumber`. |
| Mermaid reserved characters (`;`, `#`, `{}` in flowchart labels) | High | Quote labels, avoid `;`, and run the parse check before completion. |
