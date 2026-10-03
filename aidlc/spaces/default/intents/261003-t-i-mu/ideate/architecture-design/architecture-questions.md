# Architecture Design — Questions

Mode: YOLO. Recommended answers recorded; each maps to an ADR in `docs/adr/`.

## 1. Overall shape (auth model)?

- A. Kratos sessions + Keto + Go API (no OAuth2 server) — **recommended**
- B. Kratos + Hydra, OAuth2/OIDC for both clients
- C. Kratos + Oathkeeper gateway in front of the API
- D. Go BFF proxies all auth for both clients
- X. Other

[Answer]: A — Recommendation (ADR-0002, ADR-0006). B deferred until third-party clients; C until a second backend service.

## 2. Admin vs customer separation?

- A. One Kratos, two identity schemas (`admin` non-selectable) + Keto roles — **recommended**
- B. Two Kratos instances
- C. One schema + role in metadata
- X. Other

[Answer]: A — Recommendation (ADR-0004).

## 3. Scale & availability target?

- A. Single region, 2+ replicas per stateless component, managed Postgres with PITR; ~10k customers, < 50 RPS — **recommended**
- X. Other

[Answer]: A — Recommendation (assumption; revisit with real numbers).

## 4. Data ownership & deployment topology?

- A. One Postgres cluster, DB per component; only Kratos public + identity-service public; everything else private — **recommended**
- X. Other

[Answer]: A — Recommendation (ADR-0007, docs/architecture/05-deployment.md).

## 5. Consistency for revocation?

- A. ≤ 30 s eventual (in-process cache) — **recommended**
- B. Immediate (no cache / shared cache)
- X. Other

[Answer]: A — Recommendation; accepted risk recorded in security doc §6.7.
