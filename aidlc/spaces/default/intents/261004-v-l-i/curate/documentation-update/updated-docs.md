# Updated Docs

## New

- `docs/features/README.md`: system context diagram, use-case overview,
  catalogue of F01–F16, legend, and how to re-check the diagrams.
- `docs/features/F01…F16`: 16 feature pages, each with a functional flowchart,
  sequence diagrams, an errors table, side effects and code references.

## Changed

- `docs/README.md`: added a note that the TL;DR context diagram is the v0.1
  design, plus reading-order row 16 linking to Feature diagrams.
- `docs/architecture/01, 03, 06, 08, 09, 10`: added a "Current diagrams"
  pointer to the matching feature pages.
- `docs/architecture/03-auth-flows.md` §3.3 and
  `10-pseudonymous-login.md` §10.4: fixed `;` in message text, so both blocks
  now render on GitHub.

## Known stale content left for a follow-up

The code traces confirmed these. They are recorded so a future docs pass can
fix them:

- `docs/README.md` TL;DR still says "no code yet" and "No Hydra in v1". It
  also says mobile uses Kratos native flows for login and registration, but
  since ADR-0014 these go through identity-service.
- 03 §3.2: the `/v1/me` response shape is out of date. It should be
  `login{type,value}`.
- 03 §3.5: shows session age > 12 h → 403. The code returns 401 and revokes
  the session.
- 03 §3.6: the invite order is wrong.
- 04 §4.4: the grants are shown at table level; they are actually per column.
- 06 §6.7: says "enrolment is audited", but nothing is written to the audit
  log.
- 08 §8.7: understates the scope of the OpenBao dependency.
- 09 §9.4: `tokens_valid_after` value.
- 10 §10.2: courier order.
- OpenAPI `/v1/auth/login` text: describes the account limiter as counting
  failed attempts, but it counts every attempt.
