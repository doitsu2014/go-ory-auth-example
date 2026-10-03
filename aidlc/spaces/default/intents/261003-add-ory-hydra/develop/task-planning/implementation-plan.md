# Implementation Plan — M2M with Hydra

1. Lead: H1 infra. Verify that `make up` is healthy, that Hydra issues a JWT
   to a test client, and that the admin port is reachable only on 127.0.0.1.
2. Go developer: H2 + H3, after the security design review findings are
   folded into the spec (§6). Verify with build, vet, `test -race`,
   integration and `make up`.
3. React developer: H4, in parallel with step 2.
4. Lead: H5. Then code review: one security reviewer and one correctness reviewer.
