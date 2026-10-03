# Alternatives Analysis

The Ory stack is a fixed constraint (C-03); this table records why, and what
the realistic alternatives would cost, so the decision stays defensible.

| Option | Fit | Pros | Cons |
| --- | --- | --- | --- |
| **Ory self-hosted (Kratos + Keto)** — chosen | High | Apache-2.0, Go-native, headless (own UI in React & Flutter), Postgres-backed, native mobile flows | We operate it: upgrades, migrations, email courier, secrets |
| Ory Network (managed) | High | Same APIs, no ops, console | Cost; data residency outside our control; same code works if we switch later |
| Keycloak | Medium | Batteries-included admin UI, OIDC everywhere | Java, themed server-rendered UI, OAuth2-only for mobile (system browser) |
| Zitadel / Authentik | Medium | Go (Zitadel), modern | Less headless; OIDC-centric |
| Auth0 / Firebase Auth / Cognito | Medium | Zero ops | Vendor lock-in, per-MAU cost, contradicts "Ory stack" |
| Roll our own in Go | Low | Full control | Highest security risk; violates "don't hand-roll auth" |

Conclusion: self-hosted Ory, written so that switching to Ory Network is a
configuration change (base URLs + API key), not a rewrite.
