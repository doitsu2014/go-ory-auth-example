# Intent Capture — Questions

Mode: YOLO (question budget 0). Recommended answers recorded; the owner may override.

## 1. Which data counts as "user information" to encrypt?

- A. New customer personal information (phone, date of birth, address, national ID) plus the existing `display_name`, stored by identity-service
- B. Also Kratos traits (email, name)
- X. Other

[Answer]: A — Recommendation. Kratos must read the email in clear to log in; Kratos tables are protected by volume encryption and least privilege, and minimising traits is a follow-up.

## 2. What is the "world-standard flow"?

- A. Envelope encryption (NIST SP 800-57 / 800-38D AES-GCM), KEK in a KMS, per-subject DEK, blind index for lookup, masking + audited just-in-time reveal, crypto-shredding for erasure (GDPR Art. 17/32, Vietnam Decree 13/2023, OWASP Cryptographic Storage)
- B. Postgres `pgcrypto` with a key in the app config
- C. Disk encryption only
- X. Other

[Answer]: A — Recommendation. B leaks keys into SQL logs; C does not protect against SQL-level access.

## 3. Which key manager runs locally?

- A. OpenBao (open-source Vault fork) Transit engine in Docker Compose, behind a KEK-provider interface so AWS/GCP KMS can be added
- B. A static local key only
- X. Other

[Answer]: A — Recommendation. Matches how production KMS works (wrap/unwrap, rotation) while staying open source.
