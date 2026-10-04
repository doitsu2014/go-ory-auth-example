# Test plan

| Req | Level | Test |
| --- | --- | --- |
| PLX-FR-01 | unit | `TestPLXFR01_LoginWithAddress` (handle used, decoy for unknown, identical rejection); HTTP `TestPLXFR01_HTTPRegisterAndLogin` |
| PLX-FR-01 | smoke | login with an upper-case, padded email; wrong password and unknown address give identical problems |
| PLX-FR-02 | unit / HTTP / smoke | `TestPLXFR02_RegistrationStoresSealedUnderRandomHandle`; duplicate rejected with 4000007 |
| PLX-FR-03 | unit | `TestPLXFR03_RecoveryNeverHandsOutTheFlow` (sealed ref; wrong, forged and tampered refs; grant redaction); HTTP `TestPLXFR03_HTTPRecovery` (200 / 410) |
| PLX-FR-03 | smoke | no flow id in the response; forged id gets 422; wrong code gets 4060006; grant; settings; login with the new password |
| PLX-FR-04 | integration | postgres `TestPLIFR02_LoginIdentifierRepo` (lookup key unique, `GetByLookupKey`); smoke checks the opaque handle |
| PLX-FR-05 | HTTP / smoke | `/v1/auth/identifiers` returns 404 |
| PLX-FR-06 | unit / HTTP | `TestPLXFR01_SelfServiceLogin` (the identifier node value is dropped); `TestPLXFR06_HTTPRejectionCarriesMessageIDsOnly` |
| PLX-FR-07 | unit | `TestPLXFR07_LegacyCustomerSignsInDuringTransition`, `TestPLXFR07_LegacyRecoveryWithStrayUnboundRow`; migration tests |
| PLX-FR-08 | Flutter | repository, client, screen and integration tests (sign-in, sign-up, verify, recovery, phone) |
| PLX-FR-09 | unit | `TestPLIFR11_FindCustomers`, `TestPLIFR12_Reveal…` (lookup through the vault) |
| PLX-NFR-01 | unit / HTTP | log-leak cleanup checks the password and the address; strict bodies; `AuthSession` and `RecoveryGrant` redaction |
| PLX-NFR-02 | unit | `TestPLXNFR02_RateLimits`, `…FailedSignInsPerAccount`, `…AccountLimitHoldsUnderConcurrency`, `TestSECC03_…` |
| PLX-NFR-03 | unit | decoy path (`Login`, `StartRecovery`) |
| PLX-NFR-04 | adapter | `TestPLXNFR04_SelfServiceFailures` (5xx → 503, a 400 without a flow, a missing token) |
| PLX-NFR-05 | manual + smoke | a customer registered with the old build signs in after the upgrade; seed finds the existing customers |
