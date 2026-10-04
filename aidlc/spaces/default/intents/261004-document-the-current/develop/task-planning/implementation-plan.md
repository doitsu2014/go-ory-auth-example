# Implementation Plan

1. Re-read the implementation for exact names:
   - `app/login.go`, `courier.go`, `personalinfo.go`;
   - `httpapi/router.go`;
   - `deploy/ory/kratos/webhooks/*.jsonnet`, `kratos.yml.tmpl`;
   - OpenBao policies.
2. Write D1 in English, in the style of chapters 01–09 (tables, short
   paragraphs, Mermaid).
3. D2 links.
4. Validate the Mermaid with `@mermaid-js/mermaid-cli` (scratchpad). If that
   is unavailable, use a careful syntax review.
5. No code changes; the CI docs impact is none.
