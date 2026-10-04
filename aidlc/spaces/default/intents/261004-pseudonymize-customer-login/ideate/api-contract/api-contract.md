# API Contract — Pseudonymous customer login identifiers

Source of truth after Develop: `api/openapi/identity-service.v1.yaml`
(contract-first, ADR-0010). This document specifies the delta. All errors are
RFC 9457 `application/problem+json` with a stable `code`.

## 1. Interfaces

| # | Interface | Kind | Auth | Change |
| --- | --- | --- | --- | --- |
| I1 | `POST /v1/auth/identifiers` | public REST | none (IP-limited) | **new** |
| I2 | `GET /v1/me` | customer REST | bearer | `login` added; `email` optional + deprecated |
| I3 | `GET /admin/v1/customers` | admin REST | cookie, AAL2 | `?email=` **removed**; items gain `login` (masked) |
| I4 | `GET /admin/v1/customers/{id}` | admin REST | cookie, AAL2 | gains `login` (masked) |
| I5 | `POST /admin/v1/customers/lookup` | admin REST | cookie, AAL2 | accepts `login` **or** `phone_number` |
| I6 | `POST /admin/v1/customers/{id}/personal-info/reveal` | admin REST | cookie, AAL2 | `fields` gains `login`; response gains `login` |
| I7 | `GET /m2m/v1/customers/{id}` | machine | JWT | unchanged (never exposed contact) |
| I8 | `POST /internal/hooks/kratos/pre-registration` | Kratos webhook (:8081) | webhook API key | **new** (`response.parse: true`) |
| I9 | `POST /internal/hooks/kratos/after-registration` | Kratos webhook | webhook API key | body gains `login_id`; binds the vault row |
| I10 | `POST /internal/hooks/kratos/courier` | Kratos courier http (:8081) | **courier** API key | **new** |
| I11 | Kratos identity schema `customer` | data schema | — | `customer.v2.transition.json` → `customer.v2.json` |
| I12 | CLI `identity-service pii migrate-kratos-logins`, `pii purge-unbound-logins`, `pii rekey-logins` (procedure only), `keys rewrap` (extended) | CLI | operator shell | **new / extended** |
| I13 | Operator SQL `deploy/ory/kratos/scrub/scrub-courier.sql` | ops script | kratos DB role | **new** |
| I14 | Audit actions | event | — | `customer.login.migrated`, `customer.login.erased`, `customer.login.lookup`, `customer.login.revealed` is folded into `customer.pii.revealed` (fields list) |

## 2. Shared schemas

```yaml
LoginType:
  type: string
  enum: [email, phone]

LoginIdentifier:          # full value — owner (/v1/me) and reveal only
  type: object
  additionalProperties: false
  required: [type, value]
  properties:
    type: { $ref: LoginType }
    value: { type: string, maxLength: 320, description: "Email lower-cased, or phone E.164" }

MaskedLogin:              # admin views
  type: object
  required: [type, masked]
  properties:
    type: { $ref: LoginType }
    masked: { type: string, example: "a***@e***.com" }
```

Masking rules (domain, fixed shape so length does not leak):
- email: first char of local part + `***` + `@` + first char of the first
  domain label + `***` + `.` + last label (TLD). `alice@example.com.vn` →
  `a***@e***.vn`.
- phone: country code + `*******` + last 3 digits: `+84*******567`.

## 3. I1 — Resolve a login identifier

`POST /v1/auth/identifiers` — no credential. Rate limits (per client IP from
the trusted hop; IPv6 by /64): `sign_in|recovery|verification` 20/min and
200/day; `registration` 5/min and 30/day; global new-vault-insert cap
120/min. `Cache-Control: no-store`.

Request (`additionalProperties: false`, unknown fields → 400 `invalid_request`):

```json
{ "type": "email", "value": "  Alice@Example.com ", "purpose": "registration" }
```

| Field | Rules |
| --- | --- |
| `type` | `email` \| `phone` |
| `value` | string ≤ 320. email: trimmed, lower-cased, `net/mail` strict, no control/format chars, ≤ 254 after normalisation. phone: digits with optional `+`, spaces, `-`, `.`, `(`, `)`; a leading `0` is read as national (default region `VN`); result E.164 `^\+[1-9][0-9]{7,14}$`; country code must be in `LOGIN_PHONE_ALLOWED_COUNTRIES` |
| `purpose` | `registration` \| `sign_in` \| `recovery` \| `verification` |

200:

```json
{ "identifier": "l4cwc5fmnvxqxufy7wuuh2mfathke4fvwo3curj5ydaoo3iijsgq@login.invalid" }
```

The response never says whether an account or vault entry exists.

Errors:

| Status | code | When |
| --- | --- | --- |
| 400 | `invalid_request` | malformed JSON / unknown field |
| 422 | `validation_failed` | `errors: [{field: "value", code: "invalid_format" \| "too_long" \| "invalid_characters" \| "unsupported_country"}, {field: "type"\|"purpose", code: "invalid"}]` — **the value is never echoed** |
| 429 | `rate_limited` | `Retry-After` set |
| 503 | `dependency_unavailable` | key manager or DB down |

```json
{ "type": "https://docs.go-ory-auth-example.local/problems/validation-failed",
  "title": "Validation failed", "status": 422, "code": "validation_failed",
  "errors": [{ "field": "value", "code": "invalid_format" }] }
```

Idempotency: naturally idempotent (deterministic pseudonym; insert-if-absent).

## 4. I2 — `GET /v1/me`

```yaml
Me:
  required: [id, login, email_verified, locale, created_at]
  properties:
    login: { $ref: LoginIdentifier }
    email: { type: string, format: email, deprecated: true,
             description: "Present only when login.type = email. Use login." }
    email_verified: { type: boolean, description: "The login identifier (email or phone) is verified. Name kept for compatibility." }
```

```json
{ "id": "b8b8…", "login": { "type": "phone", "value": "+84901234567" },
  "email_verified": true, "locale": "vi-VN", "display_name": null,
  "avatar_url": null, "created_at": "2026-10-04T02:11:06Z" }
```

New failure: vault entry missing → 500 `internal` + log
`login_identifier_missing` (should not happen; repaired by purge/migration);
key manager down → 503 `dependency_unavailable`. A legacy (unmigrated)
identity returns `login: {type: email, value: <traits.email>}`.

`PATCH /v1/me` returns the same `Me`. The 403 `email_not_verified` code is
unchanged and now means "login identifier not verified".

## 5. I3/I4 — Admin customer list and detail

- `GET /admin/v1/customers?state=&page_size=&page_token=` — `email` parameter
  removed; a request carrying `email` → 400 `invalid_request` (strict params).
- `Customer` schema: `email` removed from `required`, kept optional and
  **never returned for customers**; new `login: MaskedLogin` (nullable) and
  `login_unavailable: boolean` (true when the key manager failed; list still
  renders).

```json
{ "items": [ { "id": "b8b8…", "login": { "type": "email", "masked": "a***@e***.com" },
  "login_unavailable": false, "email_verified": true, "state": "active",
  "display_name": null, "created_at": "2026-10-04T02:11:06Z" } ] }
```

## 6. I5 — Lookup

```yaml
CustomerLookupRequest:
  type: object
  additionalProperties: false
  properties:
    phone_number: { type: string }          # personal-info phone (blind index), unchanged
    login: { $ref: LoginIdentifier }        # login identifier (exact)
  # exactly one of phone_number | login → else 422 validation_failed {field: body, code: one_of}
```

`login` lookup: normalise → pseudonym → Kratos admin
`credentials_identifier=<pseudonym>` (and, while `LOGIN_MIGRATION_PHASE !=
complete`, also `=<plaintext>` for legacy identities) → customers only.
Charged 1 unit on the existing per-actor lookup limiter, audited
`customer.login.lookup` (details: `{type}` only). Result items gain
`login: MaskedLogin`; `truncated` is always false for `login`.

```json
{ "login": { "type": "phone", "value": "0901 234 567" } }
→ 200 { "truncated": false, "items": [ { "id": "b8b8…", "state": "active",
        "login": { "type": "phone", "masked": "+84*******567" },
        "personal_info": { … masked … } } ] }
```

## 7. I6 — Reveal

`RevealRequest.fields` enum gains `login`; omitted `fields` = all including
`login`. `PersonalInfo` response gains `login: LoginIdentifier` (nullable);
audit `customer.pii.revealed` lists `login` among field names. A customer
with no personal info but a login still returns 200 with only `login` set.

## 8. I8 — Pre-registration webhook

Kratos config: registration `after.password.hooks[0]` = `web_hook`,
`response.parse: true`, `can_interrupt: true`. Body Jsonnet:

```jsonnet
function(ctx) {
  schema_id: ctx.identity.schema_id,
  traits: ctx.identity.traits,
  flow_type: ctx.flow.type,
}
```

| Outcome | Response |
| --- | --- |
| schema ≠ `customer` (cannot happen, admin not selectable) | 200 `{}` |
| `traits.email` present (legacy shape) | 400 `{"messages":[{"instance_ptr":"#/traits/login_id","messages":[{"id":4049001,"type":"error","text":"Update the app to sign up."}]}]}` |
| `login_id` not a pseudonym / no vault row | 400 message id `4049002` "We could not confirm this email or phone number. Please try again." |
| legacy identity exists for the plaintext (phases 1–2) | 400 message id `4000007` (Kratos's own "An account with the same identifier exists already") |
| ok | 200 `{}`; row `last_validated_at = now()` |
| key manager / DB / Kratos admin down | 503 → Kratos shows a generic error; registration not persisted |

Custom message ids 4049001/4049002 live in an unused slot of Kratos's registration range (4040xxx) to avoid Kratos's own 40000xx validation ids; the existing population-guard id 4000001 is unchanged
(population guard); clients map them by id (vi/en).

## 9. I9 — After-registration webhook (changed)

Body adds `login_id: ctx.identity.traits.login_id`. Behaviour: provision
profile (unchanged) **and** bind the vault row to `identity_id` (A1/A6).
`response.ignore: true` stays; failures are repaired lazily on `/v1/me`.

## 10. I10 — Courier webhook

Kratos `courier.delivery_strategy: http`, `courier.http.request_config`:
`url = <webhook base>/internal/hooks/kratos/courier`, `method: POST`,
`auth: api_key Authorization: <KRATOS_COURIER_API_KEY>`, body Jsonnet:

```jsonnet
function(ctx) {
  recipient: ctx.recipient,
  template_type: ctx.template_type,
  identity_id: if std.objectHas(ctx.template_data, 'identity') then ctx.template_data.identity.id else null,
  code: if std.objectHas(ctx.template_data, 'verification_code') then ctx.template_data.verification_code
        else if std.objectHas(ctx.template_data, 'recovery_code') then ctx.template_data.recovery_code
        else if std.objectHas(ctx.template_data, 'login_code') then ctx.template_data.login_code
        else null,
  expires_in_minutes: if std.objectHas(ctx.template_data, 'expires_in_minutes') then ctx.template_data.expires_in_minutes else null,
}
```

Only these fields reach us; Kratos-rendered subject/body and URLs are never
sent (A4). Request schema (strict):

| Field | Rule |
| --- | --- |
| `recipient` | ≤ 320 |
| `template_type` | ∈ {`verification_code_valid`, `recovery_code_valid`, `verification_code_invalid`, `recovery_code_invalid`} (others → drop) |
| `identity_id` | uuid, required for `*_valid` |
| `code` | `^[0-9]{6}$` for `*_valid`, null for `*_invalid` |
| `expires_in_minutes` | 1..1440 or null |

Responses (Kratos treats non-2xx as retryable):

| Status | When |
| --- | --- |
| 204 | sent, or duplicate (dedupe hit), or **dropped permanently** (bad payload, unknown/unbindable recipient, over quota, unsupported template) — each drop increments `courier_dropped_total{reason}` |
| 401 | bad API key |
| 503 | transient: key manager, DB, Kratos admin, SMTP/SMS failure |

## 11. I11 — Kratos identity schemas

`customer.v2.transition.json` (phases 1–2): `traits` = `oneOf` [ `{login_id}`
(pattern `^[a-z2-7]{52}@login\.invalid$`, password identifier, verification
+ recovery via email), `{email}` (legacy, same extensions as v1.1) ].
`customer.v2.json` (phase 3): only `login_id`. Schema id stays `customer`.
`selfservice.methods.profile.enabled: false`.

## 12. I12 — CLI

| Command | Flags | Output | Exit |
| --- | --- | --- | --- |
| `pii migrate-kratos-logins` | `--dry-run`, `--batch 100` | `scanned=N migrated=N reverified=N skipped=N failed=N` (no values) | 0 ok, 1 any failure |
| `pii purge-unbound-logins` | `--older-than 24h`, `--dry-run` | `unbound_deleted=N rebound=N orphan_deleted=N` | 0 / 1 |
| `keys rewrap` | (existing) | adds `login_rewrapped=N` | |

## 13. Compatibility

- Versioning stays `/v1`; changes are additive except: `Me.email` /
  `Customer.email` no longer required (first-party clients updated in the same
  release), `?email=` removed from the admin list (admin web updated in the
  same release). Recorded in docs/api changelog.
- Breaking for **old mobile builds**: plaintext registration is rejected
  (4000002) and, after migration, plaintext sign-in fails (Kratos generic
  invalid-credentials). Rollout requires a minimum app version (runbook).
- Deprecation: `Me.email` removed in `/v2` or after 2 mobile releases.
