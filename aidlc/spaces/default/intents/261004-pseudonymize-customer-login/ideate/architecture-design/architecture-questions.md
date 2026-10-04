# Architecture Design — Questions

Mode: **yolo**; answers are recommendations, not asked of the human.

| # | Question | Recommendation |
| --- | --- | --- |
| 1 | Scale? | Same as today: single region, one identity-service deployment (≥ 2 replicas in prod). Resolve is O(1) Transit HMAC; no new scaling unit |
| 2 | Consistency between vault and Kratos? | Kratos holds only the pseudonym (deterministic from the address). The vault row is created **before** the Kratos identity (registration) and kept until the identity is gone. Eventual consistency is repaired by the purge job (bind or delete) — no distributed transaction |
| 3 | Availability? | Customer sign-up / sign-in / code delivery now need identity-service + OpenBao. Admin login does not. Kratos courier retries cover short outages |
| 4 | Latency? | Resolve p95 < 60 ms; one extra round trip before each customer flow submit |
| 5 | Deployment topology? | No new containers. New OpenBao keys + policy lines; Kratos courier → identity-service webhook port (:8081) on the existing private network |
| 6 | Data ownership? | identity-service owns the address (vault); Kratos owns credentials keyed by pseudonym; OpenBao owns keys |
| 7 | Integration boundary for delivery? | Kratos `http` courier → identity-service dispatcher → SMTP (email) / SMS provider (phone). identity-service becomes the only sender of customer messages |
| 8 | Where does normalisation live? | Server only (domain package). Clients send what the user typed (trimmed); the server normalises before HMAC. Mobile validates format for UX only |
