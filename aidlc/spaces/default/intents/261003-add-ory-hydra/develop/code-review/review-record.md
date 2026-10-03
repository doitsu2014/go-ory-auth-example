# Review Record — code-review (M2M with Hydra)

Reviewers:
- security-agent: design review before coding, then a code review with live
  forgery probes;
- code-reviewer-agent: Go and admin web correctness.

The implementing developers fixed every finding, and the lead re-ran all checks.

## Design review: 6 blocking, 10 advisory

All are resolved in technical-spec §6:
- the machine audit feed uses an allowlist;
- client and claim checks (`sub == client_id`, managed client, exact audience);
- rotation sets `tokens_valid_after`;
- the Hydra admin API sits on an isolated network;
- the DB provisioning job is idempotent;
- accepted risks are recorded.

## Code review findings

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| S1 | Major (live) | 61 junk bearers from one IP → valid tokens got 429 | Failure budget charged only on rejections; a valid token is never 429'd by it; startup warning for `TRUSTED_PROXY_HOPS=0` |
| C1/S3 | Major | Any Hydra 4xx was cached as "client not found" | Only 404/400; everything else → 503, never cached |
| C2 | Major | A timed-out Hydra create left an unaudited orphan client | Service-chosen `client_id`, detached 10 s create, compensating delete of that id, `possible_orphaned_service_client` log |
| C5 | Minor | Rotate commit failure after the PATCH locked the client out | Two-phase audit (`*_started` → Hydra → `*_rotated`/`*_failed`); secret still returned |
| C4/S2 | Minor (live) | `tokens_valid_after` at 1 s resolution kept a same-second token alive | `ceil(now)+1`; test for an iat equal to the rotation second |
| C3/S6 | Minor | Single-flight lookup could return a status from before Invalidate | `Forget` + generation re-check |
| S4 | Minor | Hydra admin is unauthenticated, so a forged `managed_by` was accepted | HMAC integrity tag in client metadata (`M2M_CLIENT_TAG_KEY`), checked with `hmac.Equal` |
| S5 | Minor | `/oauth2/revoke` has no effect (offline verification) | Documented runbook: rotate or delete; accepted risk |
| C6–C8 | Minor | Contract 400/422 missing; malformed header should be 400; wrong method 404 | Added; RFC 6750 `invalid_request`; 405 + `Allow` on `/m2m` |
| C9/C10 | Minor | Web: idempotency key reused after an edit; Escape discarded the secret; weak smoke check | New key on change; close requires an "I stored it" tick; smoke asserts the token was issued |

## Verification after fixes (lead, 2026-10-03)

| Check | Result |
| --- | --- |
| Go vet (both tags), `test -race`, `test -race -count=1 -tags integration` | pass (14 integration pkgs) |
| `make up` | all 7 services healthy |
| `node scripts/smoke.mjs` | 33/33 |
| Admin web lint, typecheck, test, build | pass (108 tests) |
| Mobile test / integration | 107 / 6 (unchanged) |
| OpenAPI lint | 0 errors |

## Remaining (non-blocking)

- `/v1` and `/admin/v1` still answer a wrong method with 404 (only `/m2m` answers 405).
- Rate limits are per replica.
- Hydra admin is reachable from the `hydra-db` network and host 127.0.0.1 (local only; production uses a NetworkPolicy).
- `private_key_jwt` is deferred until external partners onboard.

## Verdict

**Approve.** There are no open blocking or major findings. Merge needs the owner's review.
