# Feasibility: customer login proxy

Verdict: **feasible, no Kratos change**. Spike run against the local stack
(Kratos v26.2.0) on 2026-10-04, server to server, with `X-Forwarded-For` set.

| Probe | Result | Consequence |
| --- | --- | --- |
| `GET /self-service/login/api`, then `POST /self-service/login` with a decoy handle | 400. `ui.messages` = [4000006]. The `identifier` node **echoes the submitted handle** in `attributes.value` | The proxy must never forward the flow. It returns message ids only (PLX-FR-06) |
| `GET /self-service/recovery/api`, then `POST /self-service/recovery {method: code, email: decoy}` | 200, `state: sent_email`, message 1060003, same flow id | Unknown and known addresses look identical. The app continues the flow by id |
| `GET /self-service/registration/api`, then `POST` with a 5-character password | 400. Node `password` has message 4000032 with context `{min_length: 12}` | Field mapping `password` works. The app's default of 12 covers the context |
| API flows created server side | Not bound to the caller (no cookie or CSRF) | A flow id created by identity-service can be continued by the app (recovery code) |

## Options considered

| Option | Verdict |
| --- | --- |
| Recovery through admin `POST /admin/recovery/code {identity_id}`, as in the shared diagram | Rejected. It needs the identity id, so unknown addresses take a different path and have different timing. It bypasses Kratos's flow semantics, and identity-service would have to send links itself |
| Self-service recovery driven with the handle (chosen) | Same response for every address. Kratos's courier still delivers through identity-service. The code and the new password go straight to Kratos |
| Proxy for verification | Not needed. The signed-in owner already knows their own handle (whoami), so it is not an oracle |

## Risks

- Passwords now pass through identity-service (loses P2). Mitigation:
  PLX-NFR-01.
- Kratos sees identity-service's IP. Session devices take the client IP from
  `X-Forwarded-For`. Rate limiting moves fully to identity-service.
- Kratos timing for unknown and known handles is Kratos's own behaviour, the
  same as before.
