# Design Decisions

| # | Decision | Alternatives | Rationale |
| --- | --- | --- | --- |
| D1 | Docs-as-code with Mermaid in Markdown | PlantUML, draw.io PNG, C4 DSL | GitHub renders Mermaid natively, the docs already use it 19 times, it diffs well, and there is no build step |
| D2 | **Use-case style** functional overview: `flowchart LR` with actors → feature nodes | UML use-case (not supported by Mermaid) | Mermaid has no use-case diagram type. A flowchart with stadium-shaped actors is the accepted idiom. |
| D3 | Per-feature **functional flowchart** of the decision logic | Activity diagram | Mermaid's flowchart is effectively an activity diagram. Diamonds show the authz and validation gates. |
| D4 | Per-feature **sequence diagram** with `autonumber`, `alt`/`opt`/`par`, and `rect` to group "inside identity-service" | One giant system sequence | Numbered steps can be cited in review. Grouping keeps each lane readable. |
| D5 | Template page structure (see architecture-doc) | Free-form | Gives every feature the same look and makes completeness checkable by script |
| D6 | Automated parse check (`mermaid.parse` under jsdom) | Manual preview | It caught 2 broken diagrams already. Reuse it as the test suite. |
| D7 | `file:line` references at the end of each page, not inside diagrams | Inline in notes | Keeps the diagrams uncluttered while staying traceable |
