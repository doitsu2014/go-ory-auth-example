# Requirements: customer login proxy (PLX)

Prefix `PLX`. These requirements supersede PLI-FR-01 (resolve endpoint), DD-18
(device pseudonym cache) and the deterministic identifier part of
PLI-FR-02 / ADR-0013. All other PLI requirements still apply.

## Functional

| ID | Requirement | Acceptance criterion |
| --- | --- | --- |
| PLX-FR-01 | `POST /v1/auth/login {login:{type,value}, password}` signs a customer in through Kratos native API flows driven by identity-service | 200 `{session_token, session}`; wrong password or unknown account → 400 `auth_flow_rejected` with `errors[{field:"form", code:"4000006"}]`, the same for both |
| PLX-FR-02 | `POST /v1/auth/registration {login, password}` registers a customer | 200 `{session_token, session, verification_flow_id?}`; a vault row exists for the address; Kratos `traits.login_id` is the row's handle |
| PLX-FR-03 | `POST /v1/auth/recovery {login}` starts a Kratos code recovery flow | 200 `{flow_id}` with the same shape for known and unknown addresses; a code is delivered only for known ones; the app submits the code and the new password to Kratos directly |
| PLX-FR-04 | New logins get a **random** handle; the HMAC of the address is stored separately as `lookup_key` | migration 0007; existing rows `lookup_key = pseudonym` (no Kratos change) |
| PLX-FR-05 | `POST /v1/auth/identifiers` is removed | 404; no route or policy remains |
| PLX-FR-06 | Kratos flow errors are returned as Kratos message ids only, mapped to fields `login`, `password` or `form` | the response never contains a flow, node value, Kratos text or handle |
| PLX-FR-07 | While the migration phase is `transition`, legacy customers (plaintext email trait) still sign in and recover through the proxy | legacy login works through the proxy |
| PLX-FR-08 | The mobile app signs in, signs up and starts recovery through the proxy; verification, recovery code, settings and logout stay direct to Kratos | one call to sign in; no pseudonym cache; app tests pass |
| PLX-FR-09 | Admin lookup finds customers through the lookup key | the admin lookup test still passes |

## Non-functional

| ID | Requirement | Acceptance criterion |
| --- | --- | --- |
| PLX-NFR-01 | Passwords pass through identity-service only in memory: never logged, never stored, never in errors | log-redaction key `password` set; adapter errors are value-free; strict request bodies |
| PLX-NFR-02 | Rate limits: per IP and per /24·/48 network for every route; registration has its own per-IP limit; failed sign-ins per account (lookup key) | 429 `rate_limited` with `Retry-After` |
| PLX-NFR-03 | An unknown address costs the same calls as a known one (HMAC, vault read, Kratos create and submit with a decoy handle) | code path test |
| PLX-NFR-04 | Kratos self-service calls get a timeout that allows bcrypt and HIBP checks | 10 s client |
| PLX-NFR-05 | No Kratos schema or identity migration for existing customers | smoke test signs in a customer created before 0007 |
