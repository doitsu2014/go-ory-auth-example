# Implementation Plan — Encrypted personal information

1. **Lead — P1 infra** (`deploy/openbao/*`, compose, Makefile, Keto OPL). Verify:
   `make up` healthy, `docker compose restart openbao` + `openbao-init` re-unseals,
   app token denied `rotate`/`read`.
2. **Go developer — P2–P4** in `services/identity-service`, against the
   OpenAPI contract and technical spec. Verify: `go build ./...`, `go vet`,
   `go test -race ./...`, `go test -race -tags integration ./...` against the
   compose stack (Postgres + OpenBao), `make up` image rebuild.
3. **Flutter developer — P5** and **React developer — P6** in parallel once the
   contract is frozen (now). Verify with each app's analyze/lint, typecheck, tests.
4. **Lead — P7**: smoke checks (+4), docs, ADR-0011.
5. Code review: Go correctness, security (live probes on OpenBao/DB), clients.

Security design review findings (security-agent) are folded into the spec
before step 2 starts.
