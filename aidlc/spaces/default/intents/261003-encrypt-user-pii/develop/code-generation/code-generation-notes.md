# Code Generation Notes: Encrypted personal information

## Go: `services/identity-service` (units P2–P4)

### Files

New:

| File | Purpose |
| --- | --- |
| `db/migrations/0003_customer_pii.sql` | `subject_key` (+ `UNIQUE (identity_id, key_id)`), `customer_pii` with a composite FK `(identity_id, key_id) → subject_key ON DELETE CASCADE`, CHECKs pairing the phone ciphertext, blind index and its version. Grants per §9 A17: `subject_key` SELECT/INSERT/UPDATE/DELETE; `customer_pii` SELECT/INSERT/UPDATE (no DELETE) |
| `db/queries/pii.sql` → `internal/adapter/postgres/sqlcgen/pii.sql.go` | Get/Insert (ON CONFLICT DO NOTHING)/Delete (RETURNING key_id)/ListForRewrap/UpdateWrappedDEK (optimistic)/DeleteErasedSubjectKeys (erasure ledger), Upsert/Get/FindByPhoneBidx (LIMIT) |
| `internal/domain/pii/pii.go` | `PersonalInfo`, `Address`, `NationalID`, civil `Date`; `Normalize`, `Validate(now)` (field + code only), fixed-width `Mask` (§9 A12), `FieldNames`, `Only`, plaintext `Encode`/`Decode`. Every value type redacts itself through `fmt.Formatter`, `Stringer`, `slog.LogValuer` and `MarshalJSON`. ISO 3166-1 alpha-2 list (249 codes) |
| `internal/crypto/envelope/envelope.go` | AES-256-GCM, `0x01 ‖ nonce ‖ ct ‖ tag`; AAD = `"identity-service/pii/v1\x00" ‖ 0x01 ‖ column ‖ identity_id` (§9 A7); `ErrDecrypt`; `Zero` |
| `internal/app/dekcache.go` | LRU + TTL keyed by `key_id`, copy-in/copy-out, zeroed on evict, expiry, replace and purge; injectable clock; `SweepExpired` |
| `internal/app/ratelimit.go` | Per-actor in-memory sliding log; `LookupRateRules` (30/min, 200/day) and `RevealRateRules` (20/h) |
| `internal/app/personalinfo.go` | `PersonalInfoService`: `GetMine`, `PutMine`, `EraseMine`, `GetMasked`, `Reveal`, `LookupByPhone` |
| `internal/app/keys.go` | `KeyRotationService.Rewrap` (unwrap + wrap with the same AD, optimistic UPDATE, skips keys already at the newest version) and `ReapplyErasures` |
| `internal/adapter/openbao/openbao.go` | Hand-written Transit client: encrypt/decrypt with `associated_data`, hmac `sha2-256` with `key_version=1`, `renew-self` + `RenewLoop` (TTL/2, retries every 30 s); the token file is re-read whenever its mtime or size changes, and a 403 forces a re-read; errors carry no bodies |
| `internal/adapter/localkms/localkms.go` | AES-GCM KEK (versioned, `Rotate`) + HMAC index key; failure injection for tests |
| `internal/adapter/postgres/pii.go` | `SubjectKeyRepo`, `CustomerPIIRepo`; `piiErr` keeps only SQLSTATE + constraint name and maps 23503/23505 to `ErrConflict` |
| `internal/adapter/httpapi/personalinfo.go` | 6 strict-server handlers + wire conversions (unset fields are explicit `null`) |
| Tests | `envelope_test.go`, `pii_test.go`, `localkms_test.go`, `openbao_test.go` (fake Transit), `app/personalinfo_test.go`, `httpapi/personalinfo_test.go`; integration: `openbao_integration_test.go`, `postgres/pii_integration_test.go`, `e2e/pii_integration_test.go` |

Changed: `api.gen.go` (regenerated), `sqlcgen/*` (regenerated), `sqlc.yaml` (nullable `int4` → `*int32`), `app/ports.go` (`KeyContext`, `Wrapped`, `BlindIndex`, `KeyManager`, `SubjectKey(Repo)`, `EncryptedPII`, `CustomerPIIRepo`, `Repos.SubjectKeys/PersonalInfo`), `app/errors.go` (`ErrRateLimited`, `ErrDataIntegrity`), `domain/identity` (`PermRevealCustomerPII`, added to `AllPermissions`, so `/admin/v1/me` reports it), `domain/audit` (4 actions), `httpapi/{policy,problem,server}.go` (6 route policies, `rate_limited` → 429), `platform/config.go` (`APP_ENV` + `PII_*` + `ValidatePII`), `platform/logger.go` (more redacted keys: phone, address, national_id, dek, x-vault-token, …), `postgres/repo.go`, `cmd/identity-service/main.go` (wiring, renew loop, DEK-cache sweeper, `keys rewrap [--batch]`, `pii reapply-erasures`), `testutil/fakes.go` (PII repos with FK/cascade semantics, admin role gets reveal), `testutil/itest` (`OpenBaoAddr`, `OpenBaoTokenFile`, `RepoRoot`, `RequireBaoToken`), existing tests (permission counts 4→5 / 3→4, Keto expectations, router matrix + fixture).

Outside the service: `deploy/compose/docker-compose.yml` gained `APP_ENV: local` for `identity-service` (see deviations). `deploy/openbao/*`, `apps/` and `docs/` were not touched.

### Decisions

- **Two-level AAD.** Column ciphertexts use the §9 A7 AAD. Wrapped DEKs use Transit `associated_data = identity_id "/" key_id` (`app.KeyContext`).
- **Key-manager errors** map to two sentinels. `ErrDependencyUnavailable` (→ 503) covers transport errors, 5xx/sealed, 429, 403 and a missing token. `ErrDataIntegrity` (→ 500) covers a decrypt 400 (`message authentication failed`). It also covers a GCM failure, a key mismatch or an undecodable column; these are logged as `pii_decrypt_failed` / `pii_dek_unwrap_failed` / `pii_key_mismatch` / `pii_decode_failed`, with ids and the column name only.
- **No KMS or Kratos call inside a DB transaction.** The DEK is wrapped and the blind index computed before the TX. Then one TX runs upsert + `customer.pii.updated`.
- **Erase.** One TX runs `DELETE subject_key RETURNING key_id`, which cascades, plus `customer.pii.erased`. After that the cache entry is evicted. If no key exists, nothing is audited (idempotent 204).
- **Reveal.** Authz runs first, then validation, the customer check and the quota. The service decrypts only the requested fields, keeps the plaintext as `[]byte`, commits the audit row in its own TX, and only then decodes and returns. If the audit fails, the deferred zeroing runs and an error (500) is returned. Audit details: `fields`, `reason_code`, `ticket_ref`.
- **Lookup.** Authz runs first, then phone validation, the quota (counted after validation), and the blind index (pinned version 1) with LIMIT 20. Each candidate's phone is decrypted and compared in constant time (A10). Candidates that fail integrity or are not customers are skipped. Audit `customer.pii.lookup` records `details.bidx` (hex), `matched_ids` and `matches`, with `target_id = "lookup"`. Results carry id, state and the masked view (no email).
- **Rewrap.** Transit `rewrap` takes no AD and is not in the policy, so rewrap is decrypt + encrypt. The newest version is learned from the first wrap, and keys already at it are skipped without KMS calls. Integrity failures are counted and the run continues, then exits with an error. Dependency errors abort, and the run is resumable.
- **Erasure ledger.** Deletes keys with `created_at <=` the subject's latest `customer.pii.erased` `occurred_at`. Data re-entered after an erasure therefore survives a re-run.
- **DEK cache:** a custom `container/list` implementation instead of `golang-lru/expirable`. `expirable` does not call `onEvict` on overwrite, and it hands out the shared slice, which its background expiry goroutine could zero while the key is in use.
- **`APP_ENV` defaults to `production`.** Development switches therefore fail closed: `local` KMS is allowed only for `local|test`, and `http://` OpenBao is refused in production.
- No new Go modules. Crypto uses only the stdlib.

### Deviations from the technical spec

| Spec | Implementation | Why |
| --- | --- | --- |
| §2 `KeyManager.RewrapDEK` | Not on the port; rewrap lives in `KeyRotationService` | §9 A9: Transit rewrap has no AD, so rewrap must be decrypt + encrypt |
| §2 `WrapDEK(ctx, dek)` | `WrapDEK/UnwrapDEK(ctx, KeyContext, …)` | §9 A9 subject binding |
| §1 `customer_pii` FKs | One composite FK instead of two single-column FKs; the blind-index partial index also covers `bidx_key_version`; extra CHECK `(phone_bidx IS NULL) = (bidx_key_version IS NULL)` | §9 A9; the lookup filters on the version |
| §1 `UpsertCustomerPII` etc. | `DeleteSubjectKey` returns key ids (cache eviction); added `DeleteErasedSubjectKeys` | Erase evicts the right entries; §9 B1 ledger |
| Architecture §8 code `pii_unavailable` | `dependency_unavailable` | api-contract + §9 A15 |
| api-contract: reveal `details.reason` | `reason_code` + `ticket_ref` | §9 B2 |
| Requirement AC "DELETE needs verified email" | Any authenticated customer | §9 A14 |
| "OpenBao stopped → 503" | Tested by pointing the client at an unreachable address (`127.0.0.1:1`) against the same DB | Stopping the shared container would break concurrent test packages; client behaviour is identical |
| Compose untouched by Go work | `APP_ENV: local` added to `identity-service` | Needed because `APP_ENV` defaults to production, which refuses the `http://openbao:8200` address |
| Masking table "default 2" | Complete E.164 rule (1 and 7 are 1-digit, a fixed set of 2-digit codes, all others 3-digit) | Codes are prefix-free, so no default is needed |

Accepted and documented limitations:

- A wrong-format date in the PUT body is rejected by the generated decoder as `body/invalid`, not as `date_of_birth`.
- Rate limits are per replica.
- A stale DEK stays in other replicas' caches for at most 5 min after an erase (A11).

### Verification (2026-10-03, local stack)

| Command | Result |
| --- | --- |
| `go build ./...` | ok |
| `go vet ./...`, `go vet -tags integration ./...` | ok |
| `go test -race -count=1 ./...` | ok (httpapi, kratos, localkms, openbao, app, envelope, pii, platform) |
| `go test -race -count=1 -tags integration ./...` | ok (after updating the Keto test for `reveal_customer_pii`): keto, kratos, openbao, postgres, e2e (incl. existing FR06/FR07/FR08/S1) |
| `make bao-init` | Re-applied the policies. Before it, the running OpenBao still allowed `transit/rewrap` (a stale policy from an earlier init). Afterwards the least-privilege test passes (rotate/config/rewrap/export/read → 403) |
| `make kek-rotate` | Run by `TestPIIFR09_E2E_RotateAndRewrap`; keys move to the latest version and stay readable |
| `make up` | Image rebuilt; all services healthy; `make health` ok |
| `make keys-rewrap`, `identity-service pii reapply-erasures` (in container) | Run ok (`scanned=0`, `erased_keys_deleted=0`) |
| Live check on :8080 | Fresh customer: GET 200 all-null; PUT unverified 403; DELETE 204 |

The live check did not run a verified-email PUT against the container; that path is covered in-process by the e2e tests against the same Postgres and OpenBao.

Integration coverage:

- Raw rows contain no plaintext (also hex).
- Row, column and wrapped-DEK swaps fail.
- Erase removes both rows, and the old ciphertext fails with the new key.
- Rotate + rewrap.
- Lookup.
- Reveal audit atomicity (a failing audit repo inside the real TX leaves no row and reveals nothing; the success row is committed before return).
- OpenBao unreachable → 503 while `/v1/me` stays 200.
- `identity_app` cannot `DELETE FROM customer_pii` (42501).
- The erasure ledger.
- Log capture over success and error paths in both the unit HTTP test and e2e.

### Review fixes (round 1)

| # | Fix | Test |
| --- | --- | --- |
| 1 | `PutMine` retries `keyFor` once after `ErrConflict` (upsert FK) or `ErrNotFound` (insert re-read). The erased key is evicted | `TestReview1_EraseBetweenKeyForAndUpsertRetries`, `TestReview1_InsertReReadNotFoundRetries` |
| 2 | `DEKCache.Evict` leaves a tombstone for 2×TTL; `Put` drops tombstoned ids | `TestReview2_EvictTombstoneBlocksRecache` |
| 3 | The rewrap skip compares the KEK name as well as the version (`LatestKEKName`); losing to an erase counts as a conflict | `TestReview3_RewrapComparesKEKName`, `TestReview3_RewrapLosingToEraseIsAConflict` |
| 4/6 | A missing `reason_code` returns `required`; reveal with no stored record is audited and returns all-null | `TestReview4_6_RevealReasonRequiredAndEmptyRecord`, `TestReview5_StrictPIIBodies` |
| 5 | `httpapi/bodycheck.go`, installed through the generated `ChiServerOptions.Middlewares`, checks the PUT, reveal and lookup bodies before decoding. A malformed or wrong-typed value is a 422 on that field (`invalid_format`). An unknown property is a 422 with code `unknown_field` on the enclosing object; the property name is not echoed. `updated_at` (readOnly) is accepted and ignored | `TestReview5_StrictPIIBodies` |
| 7 | Lookup fetches 21 rows and returns the first 20 with `truncated: true` when more match; it logs `pii_lookup_candidate_overflow` and audits `truncated`. The contract adds `truncated` (required) and the `phone_verified: false` descriptions; code regenerated. `items` is never null | `TestReview7_9_LookupOverflowAndWeightedQuota`, `TestReview7_LookupTruncatedField` |
| 8 | Lookup decrypts only `phone_number` per candidate; the other columns only for exact matches | `TestReview8_LookupDecryptsOnlyPhoneForNonMatches` |
| 9 | The lookup limiter is checked with `Peek(1)` before the HMAC and charged `AllowN(max(1, candidates))` before unwrapping | `TestReview7_9_…`, `TestReview9_RateLimiterAllowNAndPeek` |
| 10 | `MaskedLimiter` (`MaskedRateRules` 300/min) on `GetMasked`, checked before Kratos | `TestReview10_MaskedQuota` |
| 11 | The OpenBao client never follows redirects (also for an injected client), and 3xx → 503. A decrypt 400 is an integrity error only for "message authentication failed" / "invalid ciphertext"; any other 400 → 503. Bodies are never kept or logged | `TestReview11_RedirectNotFollowed`, `TestReview11_Decrypt400Classification` |
| 12 | https is required unless `APP_ENV` is local or test. New `PII_OPENBAO_CA_FILE` (PEM bundle, TLS ≥ 1.2) | `TestReview12_CustomCAFile`, config cases (http in staging, missing CA) |
| 13 | The reveal quota is checked before the Kratos lookup | `TestReview13_RevealQuotaBeforeKratos` |
| 14 | `Len`/`SweepExpired` nil guards. Migration `0004_pii_column_grants.sql` revokes the table-level UPDATE and grants column UPDATEs | `TestReview14_ColumnLevelUpdateGrants` (integration), `TestReview2_…` (nil) |

Verification after the fixes:

- `go build ./...` and `go vet` (both tags): ok.
- `go test -race ./...`: ok.
- `go test -race -count=1 -tags integration ./...`: ok. One earlier run hit a transient Kratos registration 500 (a Kratos-internal read timeout); the re-run passed.
- `make up`: healthy.
