# Intent: customer login through identity-service (backend proxy, opaque Kratos identifiers)

## Problem

ADR-0013 keeps customer emails and phone numbers out of Kratos by replacing
them with a deterministic pseudonym `base32(HMAC(address))@login.invalid`.
The app gets that pseudonym from the public endpoint
`POST /v1/auth/identifiers`, then talks to Kratos directly. Two cons came out
of the review of chapter 10 (§10.7):

- **C1, oracle:** anyone can turn any address into its pseudonym through the
  public endpoint. With a Kratos dump, that confirms whether a targeted address
  has an account.
- **C3, round trips:** the app needs resolve → create flow → submit, which is
  three calls per sign-in (two with the device cache).

The product owner proposed the backend-proxy model (sequence diagram shared in
chat): the app sends email/phone and password to the Identity API, which
looks up the identity and calls Kratos with an opaque identifier.

## Decision taken in chat

**Proxy + opaque id** (recommended option, chosen by the product owner on 2026-10-04):

- The app sends `{login, password}` to identity-service. identity-service
  normalises the address, computes `HMAC(address)` in OpenBao, reads the
  vault row by that **lookup key**, and drives Kratos native (API) flows on the
  server side.
- Kratos keeps `traits.login_id` in the same `…@login.invalid` shape. It is
  now a **random handle** for new logins. Existing logins keep their current
  string, so Kratos is not migrated. The vault maps `lookup_key → handle`.
- The public resolve endpoint is removed. No client ever receives a handle
  for an address it has not authenticated as.

## Scope

In scope:

- identity-service: `POST /v1/auth/login`, `POST /v1/auth/registration`,
  `POST /v1/auth/recovery` (public, rate limited). Remove
  `POST /v1/auth/identifiers`.
- Vault: a separate `lookup_key` column (migration 0007).
- Mobile app: sign-in, sign-up and forgot password go through the proxy. The
  device pseudonym cache is removed.
- Docs: ADR-0014, chapter 10, related chapters, API docs. Scripts:
  smoke and seed.

Out of scope:

- Admin web: browser flows are unchanged, and admins are not pseudonymised.
- Verification and password change: the signed-in owner already holds their
  own handle in the session, so these stay direct to Kratos.
- The `pii rekey-logins` CLI. It becomes easier (Kratos is untouched) but
  stays a follow-up.

## Success

- No unauthenticated endpoint returns a Kratos identifier.
- Sign-in is one call from the app.
- Existing customers sign in unchanged (no Kratos migration).
- Unit, integration and smoke tests pass on the local stack.
