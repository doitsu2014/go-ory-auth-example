# Feasibility Assessment — Pseudonymous customer login identifiers

## Spikes run (2026-10-04)

All spikes ran against throw-away containers (`oryd/kratos:v26.2.0` on SQLite,
`openbao/openbao:2.4.1 -dev`), not the shared compose stack. Driver scripts
lived in the session scratchpad.

| # | Question | Result | Evidence |
| --- | --- | --- | --- |
| S1 | Does Kratos accept `<base32-52>@login.invalid` for `format: email` with a `pattern`, and reject plaintext? | **Yes.** Plaintext `alice@example.com` → 400, node `traits.login_id` message id `4000004` (pattern). Pseudonym → 200, identity created | registration API |
| S1b | Is the identifier case-insensitive? | **Yes.** Login with the upper-cased pseudonym succeeds (Kratos lower-cases identifiers); base32 is emitted lower-case anyway | login API |
| S2 | What does the `http` courier send? | `{recipient, template_type, template_data{to, verification_code / recovery_code, verification_url, expires_in_minutes, identity{id, traits, …}}, subject, body, html_body, message_type, request_headers}`; `Authorization` API key header works. **No Kratos message id** in the payload | captured request |
| S2b | Unknown-recipient recovery | Response `sent_email`, **no** courier call (default `notify_unknown_recipients: false`) — no enumeration via our dispatcher | captured log |
| S3 | Pre-persist registration webhook (`response.parse: true`) | Runs **before** persist (`identity.id` is the nil UUID); a 400 with `messages[{instance_ptr:"#/traits/login_id", messages:[{id,text,type}]}]` blocks registration and is rendered on the node; identity **not** created | registration API + `identities` count |
| S4 | Admin rewrite of the trait | `PATCH /admin/identities/{id}` replacing `/traits/login_id` re-derives credential identifier, verifiable and recovery addresses; old rows are **deleted** (no residue). The new verifiable address is **unverified** | admin API + table dump |
| S4b | Preserve "verified" during rewrite | A **single** patch (trait + `/verifiable_addresses/0/verified`) does **not** work (addresses recomputed after the patch). A **second** patch `/verifiable_addresses/0/{verified,status}` does | admin API |
| S5 | Does `kratos cleanup sql` remove residual plaintext? | Only **expired** self-service flows / sessions older than `--keep-last`. **`courier_messages` and `courier_message_dispatches` are never cleaned** | table counts before/after |
| S6 | OpenBao Transit batch encrypt/decrypt with per-item `associated_data` | **Yes.** Partial failure → HTTP 400 with per-item `error` (wrong AD → `message authentication failed`) | transit API |

## Feasibility per requirement group

| Area | Verdict | Notes |
| --- | --- | --- |
| Resolve endpoint + HMAC (FR-01/02, NFR-02/08) | Feasible | Reuses `openbao` adapter pattern (`transit/hmac`); new key + policy line |
| Vault encryption (NFR-03) | Feasible | Direct Transit encrypt with AD = context string (S6); no per-entry DEK needed — the value is tiny and must be decryptable in batches for admin lists |
| Schema v2 (FR-03) | Feasible | S1 |
| Pre-persist check (FR-04) | Feasible | S3. Consequence: **registration now depends synchronously on identity-service** (it already does via the resolve call) |
| Courier dispatch (FR-05/06, NFR-10) | Feasible | S2. Dedupe key must be derived (no message id): `sha256(template_type ‖ recipient ‖ code)`. Kratos retries non-2xx |
| Admins through the http courier (NFR-11) | Feasible | The dispatcher sends non-pseudonym recipients as-is via SMTP; admin templates rendered by our dispatcher too (Kratos bodies for admins contain no customer data and may be forwarded) |
| `/v1/me`, admin list/lookup/reveal (FR-07, 10–12) | Feasible | Batch decrypt (S6) gives one round trip per page |
| Mobile (FR-08/09) | Feasible | Re-auth uses the session's `traits.login_id` (already pseudonym); display from `/v1/me` |
| Migration (FR-13) | Feasible with a **two-patch** protocol (S4b); crash between patches leaves the address unverified → vault entry records `legacy_verified` so a re-run restores it |
| Scrub (FR-14) | Feasible **only as an operator SQL script** run with the Kratos DB role (S5): our service must not read/write Kratos tables (PLI-C-03). Script deletes `courier_messages`/`dispatches` older than N and non-pseudonym rows; flows are handled by scheduled `kratos cleanup sql --keep-last 24h` |
| Phone (SMS) | Feasible | Same Kratos `email` channel for both types; dispatcher picks SMS for `type = phone`. Local SMS sink → Mailpit |

## Cost / schedule (ballpark)

| Unit | Effort |
| --- | --- |
| Go: pseudonym + vault + resolve API + courier dispatcher + SMS port + webhooks | 3–4 d |
| Go: admin list/lookup/reveal + `/v1/me` + migration/purge CLI | 2–3 d |
| Kratos/compose/OpenBao config + scrub script | 1 d |
| Mobile (4 screens + repo + tests) | 2–3 d |
| Admin web (lookup + masked contact) | 1 d |
| Docs/ADR, integration + e2e tests | 2 d |
| **Total** | ~11–14 engineer-days |

## Risks (summary; full register in constraint-register.md)

1. Registration/sign-in availability now depends on identity-service + OpenBao (R-01).
2. Old mobile builds send plaintext → rejected by schema (good) but the
   plaintext sits in the failed flow until cleanup (R-04).
3. Password similarity check now compares against the pseudonym (R-06).
4. Resolve endpoint as an oracle (R-02) — accepted by design (PLI-A-01).

## Recommendation

**Proceed.** Every blocking assumption (PLI-A-02, A-03, A-04) was verified on
the pinned versions; the two surprises (S4b two-patch, S5 courier messages
not cleaned) are absorbed into the design and recorded as constraints.
