# Intent Capture — Questions

Mode: **yolo**. Answers are recommendations; the human's input is the request
"Vẽ lại mô hình hiện tại giúp tôi và cập nhật vào docs, nêu luôn Pros and Cons
(nếu có) cho mô hình hiện tại và hướng phát triển thêm", asked right after
comparing a proposed proxy-style diagram with the implemented model.

| # | Question | Recommendation |
| --- | --- | --- |
| 1 | Problem? | The implemented ADR-0013 model is spread over 03/08/ADR-0013 and differs from the diagram the owner had in mind (backend proxy + UUID identifier). One page should draw the real model, in the same style as that diagram, so the team can compare |
| 2 | Format? | Mermaid sequence diagrams in Markdown, rendered by GitHub and the docs. Same lanes as the owner's diagram: Client, Identity API, OpenBao, PII Vault DB, Ory Kratos, Email/SMS provider |
| 3 | Which flows? | Login, registration, `/me`, forgot password (the four in the owner's diagram), plus code delivery (courier) and admin lookup/reveal, plus a per-hop API summary |
| 4 | Pros/cons? | Yes: model pros/cons, and a side-by-side with the proxy + UUID alternative |
| 5 | Future directions? | Yes, prioritised, with what each would change |
| 6 | Language? | English, like every existing doc; the summary to the owner in Vietnamese |
| 7 | Out of scope | Code changes |
