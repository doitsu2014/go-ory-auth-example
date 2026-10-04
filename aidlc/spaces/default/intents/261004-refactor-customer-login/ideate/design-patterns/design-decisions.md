# Design decisions

| ID | Decision | Why |
| --- | --- | --- |
| DX-01 | A port `AuthFlows` in app, implemented by `kratos.SelfService` (ports and adapters, as for every other dependency) | Unit-test the use case with a fake; Kratos HTTP details stay in the adapter |
| DX-02 | New `CustomerAuthService` that uses `LoginIdentifierService` for the vault (`find`, `claim`) instead of growing the latter | `LoginIdentifierService` already owns vault and admin use cases; credentials handling is a separate concern |
| DX-03 | A typed error `*AuthFlowError{Messages []FlowMessage{Field, ID}}`, mapped by `classify` | Same pattern as `ValidationError`; value-free by construction |
| DX-04 | A decoy random handle for unknown addresses instead of short-circuiting | Uniform call path (PLX-NFR-03) |
| DX-05 | Candidate identifiers (the handle, then the legacy email) tried in order. Only an invalid-credentials rejection falls through | Legacy customers during the transition, without a second endpoint |
| DX-06 | Mobile: the proxy's `auth_flow_rejected` becomes a synthetic `KratosFlow` (nodes `login`, `password`, `form`) | Re-uses `FlowFormMixin`, `fieldError` and `kratosMessage` localisation unchanged |
| DX-07 | `Pseudonym` keeps its name (the Kratos handle is still a pseudonym); the HMAC becomes `LookupKey` | Minimal churn: courier, bind, purge, mask and reveal keep working on the handle |
