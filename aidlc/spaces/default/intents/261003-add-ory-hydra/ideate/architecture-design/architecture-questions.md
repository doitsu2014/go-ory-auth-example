# Architecture — Questions

Mode: YOLO.

## 1. Separate machine plane or accept JWTs on /admin/v1?
- A. Separate `/m2m/v1` plane with scope policies
- B. Accept JWTs on admin routes mapped to Keto
[Answer]: A — Recommendation. Keeps credential-bound-to-plane; admin routes keep AAL2/cookie semantics.

## 2. How to make JWTs revocable?
- A. Cached client-status lookup (30 s) + 5 min TTL
- B. Introspection per request
- C. TTL only
[Answer]: A — Recommendation. Same revocation bound as sessions with near-zero latency.
