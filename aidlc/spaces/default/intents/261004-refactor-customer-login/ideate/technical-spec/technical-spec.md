# Technical spec

Stack: Go 1.27 (chi, oapi-codegen strict server, sqlc, pgx), PostgreSQL
(skill `db-postgres`: an additive migration with backfill, then NOT NULL and
UNIQUE in one transaction), Flutter (Riverpod, dio, ory_client).

## Database: `0007_login_lookup_key.sql`

```sql
ALTER TABLE login_identifier ADD COLUMN lookup_key bytea;
UPDATE login_identifier SET lookup_key = pseudonym;          -- pre-ADR-0014 handles are their HMAC
ALTER TABLE login_identifier
  ALTER COLUMN lookup_key SET NOT NULL,
  ADD CONSTRAINT login_identifier_lookup_key_len CHECK (octet_length(lookup_key) = 32),
  ADD CONSTRAINT login_identifier_lookup_key_key UNIQUE (lookup_key);
```

The table is small (one row per customer). Goose runs the migration in a
transaction. The app role gets no UPDATE on `lookup_key`; the re-key CLI
would grant it.

## Go

- `login.LookupKey [32]byte`, `LookupInput(id)`, `NewPseudonym() (Pseudonym, error)` (crypto/rand).
- `app.LoginKeys.LookupKey(ctx, input) (login.LookupKey, error)`.
- `app.LoginIdentifierRepo.GetByLookupKey(ctx, k)`.
- `app.AuthFlows`:
  ```go
  Login(ctx, c FlowClient, identifier, password string) (AuthSession, error)
  Register(ctx, c FlowClient, loginID, password string) (AuthSession, error)
  StartRecovery(ctx, c FlowClient, email string) (flowID string, err error)
  ```
  `FlowClient{IP netip.Addr, UserAgent string}`.
  `AuthSession{Token string; Session json.RawMessage; VerificationFlowID string}` redacts itself.
- `app.CustomerAuthService{Logins, Flows, IPLimiter, RegisterLimiter, NetLimiter, AccountLimiter, InsertLimiter, Log}`.
- Config: `LOGIN_SIGNIN_RATE` (was `LOGIN_RESOLVE_RATE`), `LOGIN_NET_RATE`
  (was `LOGIN_RESOLVE_NET_RATE`), new `LOGIN_ACCOUNT_RATE=10/15m,50/24h`,
  and `LOGIN_REGISTER_RATE`, `LOGIN_INSERT_GLOBAL_RATE` (kept).
- The Kratos self-service client uses its own `http.Client{Timeout: 10s}` and sends
  `X-Forwarded-For: <client ip>` and `User-Agent`.

## Mobile

- New `core/identity/customer_auth_client.dart`: `CustomerAuthApi` and
  `HttpCustomerAuthApi`. Removed: `login_identifier_client.dart`,
  `storage/login_identifier_cache.dart`, `LoginPurpose`, `LoginTarget`,
  `PseudonymousLogin`.
- `KratosClient`: removes `createRegistrationFlow`, `submitRegistration` and
  `createRecoveryFlow`; `submitRecovery` takes only `code`. It keeps login
  (the settings refresh uses the owner's handle) and verification.
- `AuthRepository.login({login, password})`, `register({login, password})`,
  `requestRecoveryCode({login})` → `KratosFlow(id, state: sent_email)`.
  `startVerification({loginId})` and `resendVerificationCode({flowId, loginId})`.
