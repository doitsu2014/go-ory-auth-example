# Architecture — Questions

Mode: YOLO.

## 1. Where to keep the name?
- A. Same `customer_pii` row, new column under the same DEK
- B. Separate table / key
[Answer]: A — Recommendation. One DEK per customer keeps erase = one key delete.

## 2. Migration runtime?
- A. CLI subcommand run by the operator (`./dev up` locally)
- B. Automatic at service start
[Answer]: A — Recommendation. Explicit, auditable, does not slow or couple startup to Kratos admin writes.
