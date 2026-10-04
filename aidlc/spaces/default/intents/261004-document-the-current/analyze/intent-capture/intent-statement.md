# Intent Statement — Document the pseudonymous login model

## Problem

The pseudonymous login model (ADR-0013) is implemented and released on
branch `doitsu2014/sandworm`, but it is described in pieces:

- the auth-flow diagrams in 03 §3.1–3.3 and §3.11;
- the vault and runbooks in 08 §8.12;
- the ADR itself.

The owner compared it with a proxy-style diagram (backend receives the
password, Kratos keyed by UUID) and asked for the **current** model to be
drawn the same way, with pros, cons and directions for further development.

## Users

| User | Need |
| --- | --- |
| Team / reviewers | Understand the model in one page and compare it with the alternative |
| Operators | Know which service calls which API, and where the plaintext address appears |

## Success criteria

1. A new doc `docs/architecture/10-pseudonymous-login.md` contains Mermaid
   sequence diagrams for:
   - login;
   - registration with code delivery;
   - `/me`;
   - forgot password;
   - admin lookup and reveal.

   It uses the same lanes as the owner's diagram, and every step matches the
   code (paths, payloads, key names).
2. A table lists the API calls made between each pair of services.
3. Pros and cons of the model, plus a comparison table with the
   proxy + UUID model.
4. A prioritised list of future directions.
5. Linked from 01-overview, 03-auth-flows, 08-pii-protection, ADR-0013 and
   the docs index.

## Out of scope

Code or config changes.
