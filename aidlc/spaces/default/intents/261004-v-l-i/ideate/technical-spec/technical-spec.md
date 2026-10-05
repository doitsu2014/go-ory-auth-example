# Technical Specification — Feature diagram set

## Inputs

Four parallel code traces, one per area:

- customer auth;
- customer self-service, crypto and jobs;
- admin console auth, Keto and audit;
- admin customers, PII, Hydra and M2M.

Each trace returned ordered steps with `file:line`, participants, branches,
and a list of stale docs. The traces are the source of truth for the pages.

## Deliverables

| File | Diagrams (min) | Key facts that must appear |
| --- | --- | --- |
| `docs/features/README.md` | context, use-case | :8080/:8081/:9090 listeners, Hydra, OpenBao, mobile → Kratos for verification/settings/logout |
| F01 registration | flow + 2 seq | claim reuses existing handle (4000007), pre-reg ids 4049001/4049002, after-reg provisions profile then binds, courier order dedupe → identity → vault → decrypt → quota → render → send |
| F02 verification | flow + seq | app → Kratos directly, reuse `verification_flow_id`, `email_not_verified` gate |
| F03 login | flow + 2 seq | decoy, account limiter counts every attempt, after-login 4000001, session restore |
| F04 recovery | flow + seq | target selection, `recovery_id` = OpenBao-sealed flow id, settings flow straight to Kratos, abandon logout |
| F05 settings/logout | flow + 2 seq | refresh login bypasses IS limits, 30 s cache tail |
| F06 request auth | flow + 2 seq | plane dispatch, credential rules, whoami mapping, cache epoch, policy fail-closed |
| F07 profile | flow + 2 seq | lazy provisioning, lazy bind, OpenBao dependency, no audit |
| F08 PII | flow + 2 seq | DEK create race, blind index, TX audit, erase cascade + 503 erase-incomplete |
| F09 admin sign-in | flow + 3 seq | AdminGate order, 12 h age → revoke + 401, MFA deadline, 422 location change, logout fetch, sweeper |
| F10 admin mgmt | OPL + flow + 3 seq | idempotency, compensation, mail-failure audit, advisory lock + last super_admin |
| F11 customers | flow + 3 seq | audit-in-TX before Kratos, login vs phone lookup, per-candidate limiter |
| F12 PII access | flow + 2 seq | reveal limiter 20/h, audit before response, 60 s client TTL |
| F13 audit | flow + seq | `audited()` order, action catalogue, keyset cursor, client_ip not returned |
| F14 service clients | flow + 2 seq | managed tag, secret once, two-phase rotate, `tokens_valid_after` |
| F15 M2M | flow + seq | offline JWKS, status cache, scp ∩ client scopes, allowlisted audit feed |
| F16 jobs | topology + 3 seq | rewrap, login migration, reapply erasures, readyz excludes OpenBao |

## Fixes to existing docs

- `03-auth-flows.md:125` and `10-pseudonymous-login.md:173`: replace the `;`
  inside the message text so the blocks parse.
- Add a "Current diagrams" pointer to 01, 03, 06, 08, 09 and 10, and a row
  and a note in `docs/README.md`.

## Verification

- `check.mjs` parses every Mermaid block.
- A script checks that each cited path exists and each line number is within
  its file.
- Spot-check 20 anchors to confirm each line is the named function.
- A relative-link check.
