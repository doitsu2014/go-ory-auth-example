# F16 — Background jobs and operator commands

identity-service runs several goroutines inside `serve`. It also ships
one-shot CLI subcommands that operators run for key rotation, data repair and
migrations.

## Inventory

| Job | Trigger | What it does |
| --- | --- | --- |
| `purgeIdempotencyKeys` | hourly, in-process | delete `idempotency_key` older than 24 h |
| `purgeCourierDispatches` | hourly, in-process | delete `courier_dispatch` older than 24 h, update unbound-logins gauge |
| `sweepAdmins` | `ADMIN_MFA_SWEEP_INTERVAL` (5 min) | deactivate admins past MFA deadline ([F09](F09-admin-sign-in.md)) |
| `RenewLoop` | TTL/2 | OpenBao `POST /v1/auth/token/renew-self` (retry 30 s) |
| `sweepDEKCache` | 1 min | drop expired DEKs and tombstones |
| `machineVerifier.Run` | 5 min | refresh Hydra JWKS ([F15](F15-machine-to-machine-api.md)) |
| `keys rewrap` | `make kek-rotate && make keys-rewrap` | re-wrap DEKs and login ciphertexts under the newest KEK |
| `pii reapply-erasures` | manual, after DB restore | re-delete keys erased according to the audit ledger |
| `pii migrate-kratos-logins` | `make login-migrate`, `./dev` | move plaintext Kratos emails into the login vault |
| `pii migrate-kratos-names` | `./dev`, manual | move the Kratos `name` trait into `customer_pii.name_ct` |
| `pii purge-unbound-logins` | `make login-purge` | delete vault rows never bound to an identity |
| `admin bootstrap`, `clients create` | manual | first super_admin / first service client |

## Process topology

```mermaid
flowchart LR
  subgraph serve [identity-service serve]
    L1[":8080 public: /v1/auth, /v1, /admin/v1, /m2m/v1"]
    L2[":8081 Kratos webhooks"]
    L3[":9090 ops: healthz, readyz, metrics"]
    J1[purge idempotency / courier]
    J2[MFA sweeper]
    J3[OpenBao token renew]
    J4[DEK cache sweep]
    J5[JWKS refresh]
  end
  subgraph cli [one-shot CLI]
    C0[migrate up]
    C1[keys rewrap]
    C2[pii reapply-erasures]
    C3[pii migrate-kratos-logins]
    C4[pii migrate-kratos-names]
    C5[admin bootstrap / clients create]
  end
  J1 --> DB[(identity DB)]
  J2 --> KA[Kratos admin]
  J3 --> OB[OpenBao]
  J5 --> HY[Hydra]
  C0 --> DB
  C1 --> OB
  C1 --> DB
  C2 --> DB
  C3 --> KA
  C3 --> OB
  C3 --> DB
  C4 --> KA
  C4 --> OB
  C4 --> DB
  KP[Kratos public]
  KT[Keto]
  L3 -. readyz checks .-> DB
  L3 -. readyz checks .-> KA
  L3 -. readyz checks .-> KP
  L3 -. readyz checks .-> KT
```

`readyz` checks the database, Kratos public and admin, and Keto. **OpenBao is
not a readiness check.** Migrations run as a separate `identity-migrate` job,
because `MIGRATE_ON_START` defaults to `false`.

## Sequence — KEK rotation and rewrap

```mermaid
sequenceDiagram
  autonumber
  participant OP as Operator
  participant OB as OpenBao transit
  participant CLI as identity-service keys rewrap
  participant DB as identity DB

  OP->>OB: make kek-rotate → transit/keys/identity-pii-kek/rotate + identity-login-kek/rotate
  OP->>CLI: make keys-rewrap (--batch 100)
  loop pages of subject_key (keyset by key_id)
    CLI->>DB: SELECT subject_key WHERE key_id > after ORDER BY key_id LIMIT n
    alt already newest version
      CLI->>CLI: current++
    else older version
      CLI->>OB: transit/decrypt (AD identity_id/key_id)
      CLI->>OB: transit/encrypt same AD → vault:vNEW
      CLI->>DB: UPDATE subject_key SET wrapped_dek, rewrapped_at WHERE key_id AND wrapped_dek = old
      Note over CLI: 1 row → rewrapped, 0 rows → conflict, integrity error → failed
    end
  end
  CLI->>OB: login vault: batch decrypt + re-encrypt identity-login-kek
  CLI->>DB: UPDATE login_identifier value_ct (optimistic)
  CLI-->>OP: scanned= rewrapped= current= conflicts= failed= login_rewrapped=
```

## Sequence — migrate Kratos logins into the vault

```mermaid
sequenceDiagram
  autonumber
  participant CLI as pii migrate-kratos-logins
  participant KA as Kratos admin
  participant OB as OpenBao transit
  participant DB as identity DB

  loop pages of GET /admin/identities (customers)
    CLI->>KA: GET /admin/identities?page_size&page_token
    alt traits.login_id already a handle
      CLI->>DB: finish: bind + MarkLoginVerified if legacy_verified
    else legacy traits.email
      CLI->>OB: HMAC lookup_key
      CLI->>DB: GetByLookupKey or seal + INSERT login_identifier
      CLI->>DB: bind, SetLegacyVerified
      CLI->>DB: INSERT audit_event customer.login.migrated (before Kratos call)
      CLI->>KA: PATCH [remove /traits/email, add /traits/login_id]
      opt was verified
        CLI->>KA: PATCH verifiable_addresses verified=true, status=completed
      end
    end
  end
```

## Sequence — reapply erasures after restore

```mermaid
sequenceDiagram
  autonumber
  participant OP as Operator
  participant CLI as pii reapply-erasures
  participant DB as identity DB
  OP->>CLI: identity-service pii reapply-erasures
  CLI->>DB: DELETE subject_key k USING (max occurred_at of customer.pii.erased per target) WHERE k.created_at ≤ erased_at
  CLI->>DB: delete login_identifier rows with customer.login.erased
  CLI-->>OP: counts
```

## Code references

- `services/identity-service/cmd/identity-service/main.go:160` — `serve`, `:319` goroutines, `:465` rewrap, `:504` reapply-erasures, `:647` name migration, `:691` bootstrap
- `services/identity-service/internal/app/keys.go:49` — `Rewrap`, `:118` reapply
- `services/identity-service/internal/app/loginmigration.go`
- `services/identity-service/internal/app/namemigration.go:153` — `decide`, `:193` `store`
- `services/identity-service/internal/adapter/openbao/openbao.go:385` — `RenewLoop`
- `services/identity-service/internal/adapter/httpapi/router.go:330` — ops listener
- `deploy/openbao/init.sh` — rotate
- `Makefile` — `kek-rotate`, `keys-rewrap`, `login-migrate`, `login-purge`

## Related

[08 §8.6 key lifecycle](../architecture/08-pii-protection.md) ·
[05 Deployment](../architecture/05-deployment.md)
