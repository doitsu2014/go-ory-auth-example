# Requirements — Questions

Mode: YOLO. Recommended answers recorded.

## 1. Should admins be able to see full PII?
- A. Masked by default; full reveal with a permission + reason, audited (just-in-time access)
- B. Always full
- C. Never
[Answer]: A — Recommendation. Data minimisation (GDPR Art. 25) with support needs covered.

## 2. Who may reveal?
- A. `super_admins` and `admins`; not `supporters`
- B. super_admins only
[Answer]: A — Recommendation. Supporters work from masked data and lookup.

## 3. Erasure semantics?
- A. Crypto-shred the subject's DEK, delete rows, audit the erasure
- B. Soft-delete flag
[Answer]: A — Recommendation. Makes stale copies (replicas, logs of ciphertext) unreadable.

## 4. Is `display_name` encrypted?
- A. No — classified internal, used in lists; Kratos already holds the real name
- B. Yes
[Answer]: A — Recommendation. Avoids N key unwraps per list page; revisit with name-trait minimisation.
