# API Contract changes (OpenAPI `api/openapi/identity-service.v1.yaml`)

| Schema / path | Change |
| --- | --- |
| `PersonalInfo` | + `name: PersonName` (optional; `{first?, last?}`, each ≤ 100) |
| `MaskedPersonalInfo` | + `name: {first?: string, last?: string}` masked (`"A***"`) |
| `RevealRequest.fields[]` enum | + `name` |
| `CustomerLookupResult` items | masked `name` included via `MaskedPersonalInfo` |
| `Me.name`, `Customer.name` | `deprecated: true`, description: "Never returned for customers; use personal info" |
| `AdminMe.name`, `Admin.name`, `InviteAdminRequest.name` | unchanged (admins) |
| Audit action enum (if listed) | + `customer.pii.name_migrated` (not in the M2M allowlist) |

No new paths. Errors unchanged (`422 validation_failed` with `field: name.first|name.last`).
