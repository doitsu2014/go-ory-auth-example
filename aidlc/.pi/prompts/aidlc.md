---
description: Start or resume a my-aidlc workflow (Analyze, Ideate, Develop, Launch, Curate)
argument-hint: "[description | --status | --doctor | --version | --help]"
---
Use the my-aidlc skill at .pi/skills/aidlc/SKILL.md.

Run the forwarding loop exactly as the skill describes:

1. Run `my-aidlc orchestrate next $ARGUMENTS` and read the JSON directive.
2. Act on `directive.kind`:
   - `print` — do what the message says, print the output, and stop.
   - `error` — print the message verbatim and stop.
   - `done` — present the completion summary and stop.
   - `parked` — tell the user how to resume and stop.
   - `ask` — render the question, wait for the human, then follow the route.
   - `run-stage` — read the lead agent file, the stage file, and the consumed
     artifacts; ask the stage's questions; write the artifacts at the
     `produce_paths`; present the approval gate.
3. After stage work run
   `my-aidlc orchestrate report --stage <slug> --result <outcome>` (with
   `--reason` for skips and rejections), then continue from the returned
   directive.

Stay quiet between steps. Report substance — questions, gates, artifacts,
errors — not routing.

Request:
$ARGUMENTS
