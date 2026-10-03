# Intent Capture — Questions

Mode: YOLO. Recommended answers recorded; the owner may override.

## 1. What may machines access?
- A. Read-only customer status and audit events (`customers:read`, `audit:read`); no PII
- B. Everything admins can do
[Answer]: A — Recommendation. Least privilege; PII for machines needs its own reveal/audit design.

## 2. Token format?
- A. JWT access tokens (RS256), verified offline via JWKS + cached client-status check
- B. Opaque tokens + introspection on every call
[Answer]: A — Recommendation. No per-request Hydra round trip; revocation bounded by the 30 s client-status cache (same bound as sessions).

## 3. Who manages clients and how?
- A. super_admins via admin API + admin web page (secret shown once) and a CLI for bootstrap
- B. Operators edit Hydra directly
[Answer]: A — Recommendation. Audited, permission-checked, no Hydra admin access for humans.
