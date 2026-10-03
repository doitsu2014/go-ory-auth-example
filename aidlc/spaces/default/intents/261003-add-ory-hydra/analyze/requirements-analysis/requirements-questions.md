# Requirements — Questions

Mode: YOLO.

## 1. Should machine endpoints return email?
- A. No — status only; email is PII
- B. Yes
[Answer]: A — Recommendation. Matches the PII data-minimisation rules of 261003-encrypt-user-pii.

## 2. Client authentication method in v1?
- A. `client_secret_basic` now, `private_key_jwt` documented as the production upgrade
- B. `private_key_jwt` only
[Answer]: A — Recommendation. Simplest for local use; secrets are hashed by Hydra and shown once.
