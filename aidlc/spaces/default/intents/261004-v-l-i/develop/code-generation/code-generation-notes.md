# Code Generation Notes

## Deviations from spec

None.

## Findings recorded on the pages: code differs from older docs or OpenAPI

These are facts from the code traces. They are drawn as the code behaves, and
none is fixed here:

1. **Registering an address that is already registered.** `claim` reuses the
   bound handle, and Kratos then rejects it with 4000007. That rejection
   reveals the account exists (F01).
2. **Account login limiter.** It counts every attempt, including successes
   and decoys. The OpenAPI text says "failed attempts" (F03).
3. **Refresh login.** The mobile settings refresh login goes straight to
   Kratos, so it bypasses identity-service's per-IP and per-account limits and
   the decoy (F05).
4. **Rate limiters are per replica.** They are all in-memory, except the
   courier quotas, which are stored in the DB.
5. **OpenBao dependency is wider than documented.** `GET/PATCH /v1/me` also
   depends on OpenBao (login decrypt). 08 §8.7 says only the PII endpoints do
   (F07).
6. **`readyz` does not check OpenBao** (F16).
7. **AdminGate session age.** A session older than 12 h is revoked and gets
   401. 03 §3.5 shows 403 `aal2_required` (F09).
8. **Not audited.** TOTP enrolment writes no audit event, although 06 §6.7
   says "enrolment is audited" (F09/F13).
9. **Invite order.** The invite writes the DB transaction (profile, audit,
   idempotency) before sending the email. On mail failure it compensates and
   audits `admin.invitation_failed`. 03 §3.6 shows a different order (F10).
10. **`tokens_valid_after`.** The code sets `ceil(now)+1s`. 09 §9.4 says
    `now` (F14).

Items 1–3 are worth a follow-up security review. They are noted here and not
changed (out of scope).
