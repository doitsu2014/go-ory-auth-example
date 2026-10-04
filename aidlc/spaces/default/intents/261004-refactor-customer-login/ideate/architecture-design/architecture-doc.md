# Architecture: customer login proxy (ADR-0014)

## Components and responsibilities

```
Mobile ──POST /v1/auth/{login,registration,recovery}──▶ identity-service :8080 (public plane)
   │                                                         │ HMAC lookup key (OpenBao transit/hmac)
   │                                                         │ vault row by lookup_key (Postgres identity)
   │                                                         │ Kratos public API flows (server to server)
   │◀── {session_token, session} | {flow_id} ────────────────┘
   └──direct to Kratos: verification code, recovery code, settings (password), whoami, logout
```

| Layer | Change |
| --- | --- |
| domain/login | `LookupKey` (HMAC, 32 bytes). `Pseudonym` becomes the opaque Kratos handle. `NewPseudonym()` returns a random handle. `PseudonymInput` is renamed `LookupInput` (same bytes). `Purpose` is removed |
| app | `LoginKeys.LookupKey`; `LoginRecord.LookupKey`; `LoginIdentifierRepo.GetByLookupKey`. `LoginIdentifierService`: `find`, `claim`; `Resolve` and the resolve limiters are removed. New `CustomerAuthService` (Login, Register, StartRecovery) with the limiters and port `AuthFlows`. New error `*AuthFlowError` |
| adapter/kratos | New `SelfService` (public URL, 10 s client): create the API flow, then submit. It maps a 400 flow to `*AuthFlowError` (ids and fields only) |
| adapter/postgres | Migration 0007 `lookup_key`. Queries select it. `GetLoginIdentifierByLookupKey`. The insert uses `ON CONFLICT DO NOTHING` (either key) |
| adapter/httpapi | Three public routes plus policies and body checks. Problem `auth_flow_rejected` (400) with `errors[{field, code: "<kratos id>"}]` |
| openbao / localkms | `Pseudonym` is renamed `LookupKey` (same HMAC key, version 1 pinned) |
| mobile | `CustomerAuthApi` (public dio) replaces the resolver and the cache. The sign-in and sign-up screens no longer create a Kratos flow. Forgot password starts through the proxy |

## Identifier model

| Column / trait | Before (ADR-0013) | After (ADR-0014) |
| --- | --- | --- |
| `login_identifier.pseudonym` (PK) = Kratos `traits.login_id` | HMAC(address) | Random for new rows; unchanged for existing rows |
| `login_identifier.lookup_key` (unique) | n/a | HMAC(address); backfilled = pseudonym |
| AAD of `value_ct` | kind ‖ hex(pseudonym) | unchanged (stable across HMAC re-key) |

Re-keying the HMAC now only rewrites `lookup_key`: decrypt every row, then
HMAC it with v2. Kratos is untouched.

## Flows

- **Login:** validate → IP and network limits → parse → lookup key → account
  limit (failures) → identifier candidates: the row's handle, then the legacy
  email (transition only, row missing or unbound). With no candidate, a fresh
  random decoy → Kratos login. A failure records an account hit.
- **Registration:** validate → registration and network limits → parse →
  `claim` (reuse the row by lookup key, or insert one with a random handle;
  re-read on a race) → Kratos registration (pre-registration hook validates,
  after-registration hook binds).
- **Recovery:** validate → limits → parse → handle, legacy email or decoy →
  Kratos recovery API flow `{method: code, email}` → `{flow_id}`.

## Security

- C1 closed: there is no public mapping from an address to a handle. A
  Kratos dump plus the HMAC key still identifies **pre-0007** customers only.
  Rotating old handles is a follow-up.
- Passwords transit identity-service memory. The request body is strict, the
  access log never records bodies, adapter errors are value-free, and the
  `password` log key is redacted.
- Rate limits: IP (20/1m, 200/24h), registration IP (5/1m, 30/24h), network
  (300/1m, 5000/24h), account failures (10/15m, 50/24h).
