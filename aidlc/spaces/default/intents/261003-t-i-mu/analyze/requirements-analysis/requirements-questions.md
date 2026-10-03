# Requirements Analysis — Questions

Mode: YOLO. Recommended answers recorded.

## 1. Credential methods for the first iteration?

- A. Email + password, with email verification and recovery (code); TOTP/WebAuthn MFA for admins
- B. Passwordless only
- C. Social login (Google/Apple) from day one
- X. Other

[Answer]: A — Recommendation. Social login and passkeys are Could-haves.

## 2. Can admins self-register?

- A. No. Admins are invited (created through the Kratos admin API by an existing admin or a bootstrap CLI)
- B. Yes, with approval
- X. Other

[Answer]: A — Recommendation. Self-registration is a mobile-only capability.

## 3. Must customers be blocked from the admin website?

- A. Yes — the API and the admin UI both reject non-admin sessions
- B. No
- X. Other

[Answer]: A — Recommendation.

## 4. Non-functional targets?

- A. p95 < 150 ms for authenticated API calls (excluding Kratos hashing on login); local stack starts < 2 min; OWASP ASVS L2 as the security bar
- X. Other

[Answer]: A — Recommendation.

## 5. Data the identity service owns besides Ory?

- A. A user profile (display name, avatar, preferences, status) keyed by the Kratos identity id, plus an admin audit log
- B. Nothing — Ory only
- X. Other

[Answer]: A — Recommendation.
