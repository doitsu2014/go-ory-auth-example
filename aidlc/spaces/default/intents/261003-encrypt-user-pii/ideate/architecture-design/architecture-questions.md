# Architecture — Questions

Mode: YOLO. Recommended answers recorded.

## 1. Where do wrapped DEKs live?
- A. `subject_key` table in the identity DB, one row per customer
- B. OpenBao KV per customer
[Answer]: A — Recommendation. Keeps OpenBao stateless per customer and reads to one Transit call; erasure still destroys the only copy outside backups.

## 2. Who computes the blind index?
- A. OpenBao `transit/hmac` (key never in the service)
- B. Service with a key from env
[Answer]: A — Recommendation. Least privilege; one extra call only on writes and lookups.

## 3. Field-level or row-level ciphertext?
- A. One ciphertext per column with column-bound AAD
- B. One JSON blob per row
[Answer]: A — Recommendation. Context binding per field and partial updates stay cheap.
