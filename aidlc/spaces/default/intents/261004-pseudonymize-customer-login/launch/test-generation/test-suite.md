# Test Suite — Pseudonymous customer login identifiers

Executed 2026-10-04 against the compose stack rebuilt from this branch.
The owner chose to replace the running stack: `./dev up`, existing volumes,
with migration `scanned=32 migrated=32 failed=0` on the existing customers.

## Results

| Suite | Command | Result |
| --- | --- | --- |
| Go unit | `go test ./...` | all packages ok |
| Go integration + e2e | `make -C services/identity-service test-integration` (`-race -count=1 -tags integration`) | all packages ok (run twice; stable after the fixes below) |
| Smoke (real stack) | `node scripts/smoke.mjs` | **47/47 PASS** |
| Migration idempotency | `./dev migrate-logins` (second run) | `scanned=33 migrated=0 skipped=33 failed=0` |
| Seed (real flows) | `./dev up` seeding | phone customer `+84900000005` registered, verified via the SMS sink, signed in |
| Flutter unit/widget | `flutter test` | 174 passed, 1 skipped (integration tag) |
| Flutter integration (real stack) | `flutter test -P integration test/integration/kratos_native_flow_test.dart` | **7/7 passed** (email + phone: register → verify → sign out → sign in → recovery) |
| Admin web | `pnpm test --run` | 14 files, 130 tests passed |

## Defects found by the stack run (fixed)

| # | Defect | Fix |
| --- | --- | --- |
| L1 | `GET /admin/v1/customers?email=` returned 200: the generated wrapper ignores unknown query params, so deviation D7 was not actually enforced | `forbiddenQuery` check in the strict middleware → 400 `invalid_request` (field `email`, value never echoed); covered by e2e `TestFR06_E2E_AdminPlane` |
| L2 | Reveal (all fields) of a customer without any `login_id` failed with a data-integrity error | The login field is skipped when the identity has no login (`TestDD8_E2E_RevealAuditAtomicity`) |
| L3 | The old e2e expected `GET /v1/me` to work with OpenBao down | By design since ADR-0013 (`/v1/me` decrypts the login): the test now asserts 503 `dependency_unavailable` (PLI-FR-07/NFR-06) |
| L4 | Flaky e2e: the registration verification mail now arrives later (http courier), and `VerifyEmail` used its code | Helper waits for the registration mail, checks `passed_challenge`, and retries with a fresh flow |

## Traceability (requirement → tests → result)

| Req | Tests | Result |
| --- | --- | --- |
| FR-01 resolve | `TestPLIFR01_ParseEmail/ParsePhone/PseudonymRoundTrip/InputDomainSeparation`, `TestPLIFR01_ResolveDeterministicAndTyped`, `TestPLIFR01_ResolveValidation`, `TestPLIFR01_HTTPResolveIsPublic`, `TestPLIFR01_PseudonymPinsKeyVersion`; smoke PLI-FR-01 | pass |
| FR-02 persist on registration only | `TestPLIFR02_RegistrationStoresSealedOnce`, `TestPLIFR02_LoginIdentifierRepo` (integration) | pass |
| FR-03 schema | smoke "PLI-FR-03 Kratos traits hold the pseudonym only", spikes S1/S8 | pass |
| FR-04 pre-registration | `TestPLIFR04_ValidateRegistration`, `TestPLIFR04_PreRegistrationWebhook`; smoke 4049001/4049002 | pass |
| FR-05/06 courier | `TestPLIFR05_DeliversToRealAddress`, `TestPLIA4_DropDecisionTable`, `TestPLIFR05_CourierWebhook`, `TestPLIFR06_Render`, `TestPLIFR06_LocaleFromProfile`, `TestSink_*`, `TestHTTP_ProviderContract`; smoke verification (email) + SMS sink (phone); Flutter integration | pass |
| FR-07 /v1/me | `TestPLIFR16_BindAndOwn`, `TestPLI_E2E_LoginLifecycle`; smoke PLI-FR-07 (email and phone) | pass |
| FR-08/09 mobile | Flutter `auth_repository_test` (every purpose, both types, cache, single resolve on resubmit), screen tests, settings re-auth test; Flutter integration 7/7 | pass |
| FR-10 masked admin | `TestPLIFR10_MaskManyOneBatch`, `TestPLIFR10_Mask`; e2e detail; admin-web Login column tests | pass |
| FR-11 lookup | `TestPLIFR11_FindCustomers`, `TestPLIFR12_RevealIncludesLoginAndAuditsFieldName`, e2e lookup (`TestFR06_E2E_AdminPlane`, `TestPLI_E2E_LoginLifecycle`); admin-web lookup tests | pass |
| FR-12 reveal | `TestPLIFR12_RevealIncludesLoginAndAuditsFieldName`; e2e reveal + audit field `login`; admin-web reveal tests | pass |
| FR-13 migration | `TestPLIFR13_MigrateLegacyCustomers`, `TestPLIFR13_CrashBetweenPatchesIsRepaired`, `TestPLIA2_MigrationCollision`, `TestPLIFR13_ReplaceLoginTraitReadCompare`, `TestPLIFR13_MarkLoginVerifiedSecondPatch`, **`TestPLIFR13_E2E_MigrateLegacyCustomer` (real Kratos)**; real migration of 32 customers | pass |
| FR-14 scrub | `scrub-courier.sql` (psql vars typed); smoke SQL probe | pass (probe); scrub executed in release validation |
| FR-15/16 purge/bind | `TestPLIFR15_PurgeUnboundAndOrphans`, `TestSECC10_PurgeAgeFloor`, `TestPLIA10_StaleUnboundDeleteLosesToValidation` (integration) | pass |
| NFR-01 no contact in Kratos | smoke "PLI-NFR-01 … 0 rows" (email and phone); e2e Kratos identity check | pass |
| NFR-02 key separation | `TestPLINFR02_LoginKeyNamesDistinct`, `TestPLINFR02_LoginKeysAgainstOpenBao`, **`TestPLINFR02_LoginKeysLeastPrivilege` (real OpenBao, 8 paths → 403)** | pass |
| NFR-03 AEAD binding | `TestPLINFR03_AADBindsKindAndPseudonym`, `TestPLINFR03_SealOpenBatchBoundToAD`, `TestPLINFR03_RewrapAndTamper`, `TestLocalKMS_LoginKeys` | pass |
| NFR-04 no passwords | `TestPLINFR04_HTTPResolveStrictAndValueFree` (unknown field `password` → 422) | pass |
| NFR-05 limits | `TestPLINFR05_ResolveRateLimits`, `TestSECC03_InsertCapNeverBlocksSignUp`, `TestPLIA3_Quotas`, `TestPLIA3_CountryBudgetIndependent`, HTTP 429 + `Retry-After` | pass |
| NFR-06 fail closed | `TestPLINFR06_ResolveFailsClosed`, `TestPLINFR06_LoginKeysUnavailable`, `TestPLINFR06_MigrationStopsWhenKeyManagerDown`, `TestPIINFR06_E2E_OpenBaoUnreachable` | pass |
| NFR-07 no values in logs | log assertions in `newLoginEnv` cleanup (every login/courier unit test), `TestIdentifierRedacts`, `TestSECC01_TransportErrorsCarryNoQuery`, e2e log + audit capture | pass |
| NFR-08 enumeration parity | `TestPLIFR02_RegistrationStoresSealedOnce` (always seals), response shape tests | pass |
| NFR-10 idempotent courier | `TestPLINFR10_DedupeAndRetry`, `TestPLIA11_TransientOutageNeverDrainsQuota`, `TestPLIA11_PermanentRejectionIsDropped`, `TestPLINFR10_CourierDispatchReservation` (integration) | pass |
| NFR-11 admins unaffected | existing admin e2e (`TestFR06_E2E_AdminPlane`, invitations, population guard) and smoke admin checks | pass |

Flakiness: removed (L4). Time and randomness use fixed clocks in unit
tests; e2e addresses are unique per run.
