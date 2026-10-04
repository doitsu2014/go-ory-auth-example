# 0014. Customer login through identity-service with opaque Kratos handles

- Status: Accepted
- Date: 2026-10-04
- Partly supersedes: [ADR-0013](0013-pseudonymous-customer-login-identifiers.md)
  (the resolve endpoint, the deterministic handle, and the device cache DD-18)

## Context

ADR-0013 keeps customer emails and phone numbers out of Kratos. Kratos
stores only `login_id = base32(HMAC(address))@login.invalid`. The app learns
that value from the public `POST /v1/auth/identifiers` and then calls Kratos
directly. The review of chapter 10 (§10.7) found two cons that do not go away
with mitigation:

- **C1, oracle.** Anyone can turn any address into its Kratos identifier, so
  a Kratos dump plus the public endpoint confirms targeted addresses.
- **C3, round trips.** Resolve, create flow and submit: three calls per
  sign-in (two with the device cache).

The product owner proposed the backend-proxy model: the app sends the address
and password to the Identity API, which looks up an opaque id in the PII
vault and calls Kratos with it.

A spike against Kratos v26 confirmed three things:

- API flows created server side can be continued by the app with the flow
  id.
- Recovery with an unknown address answers exactly like a known one
  (`sent_email`, message 1060003).
- A rejected login flow **echoes the submitted identifier** in the
  `identifier` node value.

## Decision

- **Proxy endpoints (public, rate limited).**
  - `POST /v1/auth/login {login:{type,value}, password}` → `{session_token, session}`.
  - `POST /v1/auth/registration {login, password}` → `{session_token, session, verification_flow_id?}`.
  - `POST /v1/auth/recovery {login}` → `{recovery_id}`.
  - `POST /v1/auth/recovery/code {recovery_id, code}` →
    `{session_token, settings_flow_id}`.

  identity-service normalises the address and computes `HMAC(address)` in
  OpenBao Transit (`identity-login-pseudonym`, v1 pinned). It reads the vault
  row by that **lookup key** and drives the Kratos native **API** flow server
  to server: it creates the flow, then submits it. It forwards the client as
  `X-Forwarded-For` and `User-Agent` so Kratos records the right session
  device. The Kratos client has a 10 s timeout, which leaves room for bcrypt
  and the HaveIBeenPwned check. `POST /v1/auth/identifiers` is removed.
- **Opaque handle.** The Kratos `traits.login_id` keeps the
  `<base32>@login.invalid` shape (schema unchanged). New logins get a
  **random** 32-byte handle. Migration 0007 adds
  `login_identifier.lookup_key` (`UNIQUE`) and backfills it as
  `lookup_key = pseudonym`. Existing handles therefore stay valid and no
  Kratos identity is migrated. `pseudonym` remains the primary key and the
  Kratos handle. The AAD (kind ‖ handle), the courier, bind, purge and reveal
  are unchanged.
- **Uniform path.** An address with no vault row runs the same Kratos flow
  with a fresh random **decoy** handle. Wrong password and unknown address
  return the same `400 auth_flow_rejected` `[{field: form, code: 4000006}]`.
  During the migration `transition` phase, the legacy plaintext email is
  tried after the handle when the address has no bound row.
- **Rejections carry Kratos message ids only.** Problem
  `auth_flow_rejected` (400) carries `errors[{field, code}]` with `field`
  ∈ `login` | `password` | `form` and `code` = the Kratos message id. The
  flow, Kratos text, context and node values are never forwarded, because
  forwarding the flow would hand the handle back. An expired flow is
  `410 auth_flow_expired`.
- **Recovery uses the stock self-service code flow, driven server side, and
  the flow id never leaves identity-service in clear.** It does not use the
  admin `POST /admin/recovery/code {identity_id}` from the proposal. The
  admin call needs the identity id first, so unknown addresses would take a
  different path, with different timing and a different answer. The
  self-service flow answers `sent_email` for everyone.
  - `POST /v1/auth/recovery` returns `recovery_id`: the Kratos flow id sealed
    with the login KEK in OpenBao (Transit AEAD, associated data
    `identity-service/recovery-flow/v1`). It is opaque and differs on every
    call.
  - `POST /v1/auth/recovery/code {recovery_id, code}` opens it, submits the
    6-digit code to Kratos, and returns the privileged
    `{session_token, settings_flow_id}`. Errors:
    - wrong code: `400 auth_flow_rejected` `form`/`4060006`;
    - expired flow: `410 auth_flow_expired`;
    - forged or tampered id: `422` on field `recovery_id` (`invalid`).

    Same per-IP and per-network limits as login.
  - The app sets the new password in the settings flow at Kratos directly
    (`X-Session-Token`). It never sends the code or an address to Kratos.
  - **Found in review:** the first implementation returned the Kratos
    `flow_id`. On the stack, `GET /self-service/recovery/flows?id=<id>` is
    public and its `email` node carries the handle, so the flow id re-opened
    the oracle (address → handle).
- **Legacy recovery (transition).** When a legacy customer's address has a
  stray **unbound** vault row, identity-service asks the Kratos admin API
  whether that handle belongs to an identity. If it does not, recovery runs
  with the legacy email.
- **Direct to Kratos, unchanged:** verification (start and resend), the
  settings flow (new password after recovery, password change and refresh
  login),
  whoami and logout. These steps use a flow id, a code, or the signed-in
  owner's **own** handle from whoami, which is not an oracle.
- **Rate limits** (per replica, sliding windows):

  | Setting | Default | Scope |
  | --- | --- | --- |
  | `LOGIN_SIGNIN_RATE` | 20/1m, 200/24h | login, recovery and recovery code, per IP (IPv6 /64) |
  | `LOGIN_REGISTER_RATE` | 5/1m, 30/24h | registration, per IP |
  | `LOGIN_NET_RATE` | 300/1m, 5000/24h | all `/v1/auth/*`, per /24 (/48) |
  | `LOGIN_ACCOUNT_RATE` | 10/15m, 50/24h | **every** sign-in attempt per lookup key, recorded before Kratos is called (concurrent guesses cannot slip past) |

  `LOGIN_SIGNIN_RATE` and `LOGIN_NET_RATE` replace `LOGIN_RESOLVE_RATE` and
  `LOGIN_RESOLVE_NET_RATE`.

Diagrams: [10-pseudonymous-login](../architecture/10-pseudonymous-login.md).

## Alternatives considered

- **Keep ADR-0013 and add app attestation to the resolve endpoint.** This
  hardens C1 without closing it, and C3 remains.
- **Proxy, but keep the HMAC as the Kratos handle.** No vault read at
  sign-in, but re-keying the HMAC would still rewrite every Kratos identity.
  The vault table lives in identity-service's own database, so the extra
  read costs about a millisecond.
- **Admin recovery code** (the proposal's step 4.8). Rejected for the
  reasons above. There is also no `POST /self-service/recovery/browser
  {identity_id}` in Kratos.

## Consequences

- **C1 closed:** no unauthenticated endpoint maps an address to a handle,
  and new handles are random. One caveat: handles created before 0007 still
  equal the HMAC of their address, so a Kratos dump **plus the HMAC key**
  could still link those customers. Follow-up: rotate them with the two-patch
  Kratos rewrite.
- **C3 closed:** sign-in is one call from the app. The device pseudonym
  cache is removed.
- **Passwords now pass through identity-service memory.** This loses P2 of
  ADR-0013 and relaxes the "never through identity-service" rule for
  customers. Controls:
  - strict request bodies;
  - the access log records no bodies;
  - the `password` log key is redacted;
  - adapter errors are value-free;
  - nothing is stored.
- Customer sign-in now depends on identity-service and its database, not only
  OpenBao. Admin sign-in is unaffected.
- Kratos sees identity-service's address, so brute-force protection lives in
  identity-service (the limits above, including a per-account limit that
  Kratos OSS lacks).
- Adding customer MFA or passkeys later needs a proxy-compatible design. A
  WebAuthn origin is bound to the app and Kratos, not to identity-service.
- Re-keying the HMAC now only rewrites `lookup_key` (decrypt, then HMAC v2).
  Kratos is untouched. This eases C4.
- Old app builds that call the removed endpoint get 404, so a forced app
  update is needed.
- **Accepted residual risks:**
  - **Registration still reveals existence.** A duplicate address gets
    `4000007`, as with native Kratos. "Indistinguishable" (PLX-NFR-03)
    applies to login and recovery only.
  - **Timing.** Unknown (decoy) handles rely on Kratos's own delay for
    unknown identifiers. With bcrypt hashing, tune and verify that it
    matches a real hash check (follow-up). During the transition, an
    unbound row can cost a second Kratos login call (legacy email
    candidate).
  - **The Kratos public API stays reachable.** The app uses it for
    verification, settings and refresh login with the owner's handle. Whoever
    knows a handle can call `/self-service/login/api` directly and bypass
    identity-service limits. Follow-up: the production ingress restricts
    `/self-service/registration/api` and `/self-service/recovery/api` to
    identity-service and rate-limits `/self-service/login/api`.
  - **Lockout.** The account limit counts every attempt, so anyone can block
    sign-in to an address for up to 15 minutes. Follow-up: a combined
    account+IP key, or a CAPTCHA after the limit.

## Revisit when

- Customer MFA or passkeys are enabled.
- Kratos ships trait encryption.
- Rotating the pre-0007 handles is scheduled.
