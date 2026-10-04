# Implementation Plan

1. **U1 (traces).** Run 4 Explore agents in parallel. Each one outputs:
   - participants;
   - ordered steps with method and path;
   - branches;
   - `file:line` references;
   - stale docs.
2. **U2–U4 (pages).** Use the page template from architecture-doc. Use the
   canonical participant aliases. Draw grey `rect` blocks for each DB
   transaction. Never put `;` inside message text.
3. **U5 (catalogue).** Build the README with the system context, the use-case
   flowchart, and the catalogue table that links F01–F16.
4. **U6 (existing docs).** Make minimal edits only: replace the two
   semicolons, and add one blockquote pointer per architecture doc. Do not
   rewrite any narrative (out of scope).
5. **U7 (verification).** Run the parse script, the path and line-range
   check, a 20-anchor spot check, and the link check.

Rollback: everything lives under `docs/`, so `git checkout -- docs/` undoes
it.
