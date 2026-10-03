# Feasibility — Machine-to-machine access (Hydra)

**Verdict: Feasible.**

## Spike S-M2M-1 (oryd/hydra:v26.2.0, memory DSN, 2026-10-03)

| Check | Result |
| --- | --- |
| Create client (`grant_types=[client_credentials]`, `audience=[identity-service]`, `access_token_strategy=jwt`) | 201, `client_secret` returned once |
| Token with scope + audience | 200, `bearer`, `expires_in` 299 |
| JWT | header `RS256` + `kid`; claims `iss`, `aud=["identity-service"]`, `client_id`, `sub`=client id, `scp=["customers:read"]`, `jti`, `iat/nbf/exp`, `ext={}` |
| Token without `audience` parameter | issued with `aud: []` → **service must require the audience** |
| Unknown scope / audience | 400 `invalid_scope` / `invalid_request` |
| JWKS | 2 RSA RS256 keys at `/.well-known/jwks.json` |
| Delete client → introspect | `active:false`, but the JWT still verifies offline until `exp` → needs the client-status check |

## Risks

| Risk | Mitigation |
| --- | --- |
| JWT not revocable | 5 min TTL + cached client-status lookup (≤ 30 s) |
| JWKS rotation | refresh on unknown `kid`, rate-limited; keep old keys during rotation |
| New Go dependency for JOSE | `github.com/go-jose/go-jose/v4` (Apache-2.0, widely used); record in notes |
| Hydra DB on existing Postgres volume | idempotent one-shot provisioning job |
