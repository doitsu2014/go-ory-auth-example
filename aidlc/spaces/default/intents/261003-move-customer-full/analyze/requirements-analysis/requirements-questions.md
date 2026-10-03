# Requirements Analysis — Questions

Mode: YOLO. Recommended answers recorded.

## 1. Remove `name` from `/v1/me` and admin `Customer` responses, or deprecate?
- A. Deprecate in the contract (`deprecated: true`, never populated for customers)
- B. Remove now
[Answer]: A — Recommendation. Additive-only rule for v1 (API guidelines); clients stop reading it in this change.

## 2. Mask format for names?
- A. First rune of each part + fixed `***`
- B. Full initials of every word
[Answer]: A — Recommendation. Fixed width per part; does not leak word count or length.
