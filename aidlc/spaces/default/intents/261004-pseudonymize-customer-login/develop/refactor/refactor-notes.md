# Refactor Notes — Pseudonymous customer login identifiers

Scope kept small: only debt this intent introduced. No behaviour change.

| # | Debt | Cost if left | Change | Safety net |
| --- | --- | --- | --- | --- |
| R1 | Mobile had the pseudonym regex twice (resolver `_pseudonym` and cache `loginPseudonymPattern`), noted by the client fix agent | The two copies could drift; a format change (e.g. a domain change at re-key) would be applied to one only | A single `loginPseudonymPattern` now lives in `lib/core/identity/login_input.dart` (the domain type). The resolver and the cache both use it | `flutter analyze` (no issues), `flutter test` 174 pass / 1 skipped (resolver bad-response test and cache validation tests cover both uses) |
| R2 | `SchemaID == string(identity.KindCustomer)` repeated 5× in `app/login.go` and `app/courier.go` | Noise in security-relevant branches | `identity.Identity.IsCustomer()` in the domain, used in those two files. Other packages are left as they were (out of scope) | `go vet ./...`, `go test ./...` all ok |

Considered and not done:

- Merging the Kratos error-format writers (`writeLoginInterrupt`,
  `writeKratosFieldError`). They differ in status code and `instance_ptr`, and
  a shared helper would save about 10 lines. Not worth touching the existing
  population guard.
- Generalising the `.env` key merge into a script shared by `dev` and the
  `Makefile`. Two small loops are clearer than another script.
