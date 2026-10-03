# Intent Capture — Questions

Mode: YOLO. Recommended answers recorded; the owner may override.
The owner confirmed rows 2 and 3 of the proposal (name into encrypted PII;
display_name as nickname, fix seeds).

## 1. When does the customer enter their name?
- A. On the personal-info screen after email verification (same rule as other PII); sign-up asks only email + password
- B. At sign-up; the app holds it and writes it after verification
[Answer]: A — Recommendation. One write path with the existing "replace needs a verified email" rule; no plaintext held in the app across the verification step.

## 2. Shape of the stored name?
- A. Structured `{first, last}` (same as the current Kratos trait)
- B. One `full_name` string
[Answer]: A — Recommendation. Lossless migration from Kratos; display order (vi: last first, en: first last) is a UI concern.

## 3. What happens to existing Kratos names?
- A. Migration CLI: copy to encrypted PII (unless PII already has a name or the customer erased PII), then remove from Kratos
- B. Drop them
[Answer]: A — Recommendation. No data loss; respects earlier erasure requests.
