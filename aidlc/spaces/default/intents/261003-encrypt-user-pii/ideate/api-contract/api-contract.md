# API Contract — Encrypted personal information

Source of truth: `api/openapi/identity-service.v1.yaml` (OpenAPI 3.0.3, Redocly
lint: 0 errors; the 8 warnings predate this change).

| Method | Path | Auth | Permission | Success | Notable errors |
| --- | --- | --- | --- | --- | --- |
| GET | `/v1/me/personal-info` | Bearer | self (customer) | 200 `PersonalInfo` | 503 `dependency_unavailable` |
| PUT | `/v1/me/personal-info` | Bearer | self + verified email | 200 `PersonalInfo` | 403 `email_not_verified`, 422 `validation_failed`, 503 |
| DELETE | `/v1/me/personal-info` | Bearer | self (any authenticated customer) | 204 | 401 |
| POST | `/admin/v1/customers/lookup` | cookie, AAL2 | `view_customers` | 200 `CustomerLookupResult` (≤ 20) | 422, 503 |
| GET | `/admin/v1/customers/{id}/personal-info` | cookie, AAL2 | `view_customers` | 200 `MaskedPersonalInfo` | 404 (not a customer), 503 |
| POST | `/admin/v1/customers/{id}/personal-info/reveal` | cookie, AAL2 | `reveal_customer_pii` | 200 `PersonalInfo` | 403, 404, 422 (reason), 503 |

## Decisions

- **PUT, not PATCH**: the form edits the whole record; clearing is explicit.
- **Errors never echo PII**: `errors[]` carries `field` + `code` only
  (`invalid_format`, `too_long`, `out_of_range`, `required`, `invalid_characters`).
  Field paths use dots: `address.country`, `national_id.number`.
- **503 reuses `dependency_unavailable`** (existing code) instead of a new
  `pii_unavailable` code; clients already handle it.
- **Lookup is a POST** so the phone number is in the body, not the URL.
- **Masked view carries `has_*` booleans** so the UI can show "not provided"
  versus "hidden".
- New `Permission` enum value `reveal_customer_pii`, returned by `/admin/v1/me`.
- New audit actions: `customer.pii.updated` (`details.fields`),
  `customer.pii.erased`, `customer.pii.revealed` (`details.fields`,
  `details.reason`), `customer.pii.lookup` (`details.matches` count only).

## Masking rules (domain)

| Field | Rule | Example |
| --- | --- | --- |
| phone | keep `+` and the country calling code (1–3 digits, longest match from a small table, default 2), mask middle, keep last 3 | `+84901234567` → `+84*******567` |
| date_of_birth | year only | `1990-05-17` → `1990-**-**` |
| address | `city` + `country` only | |
| national_id | type + `*` × (len−3) + last 3 | `079123456123` → `*********123` |
