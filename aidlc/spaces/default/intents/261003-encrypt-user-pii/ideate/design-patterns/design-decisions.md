# Design Decisions — Encrypted personal information

| # | Decision | Pattern | Why | Alternatives rejected |
| --- | --- | --- | --- | --- |
| DD-1 | `KeyManager` port with `openbao` and `local` adapters | Hexagonal port/adapter | Swap OpenBao for AWS/GCP KMS without touching the domain; `local` keeps unit tests hermetic | Calling Transit from the app layer |
| DD-2 | Per-subject DEK + Transit-wrapped | Envelope encryption | Few KMS calls, per-person crypto-shredding | Transit `encrypt` per field (one network call per field, no shredding); one global DEK |
| DD-3 | `envelope` package: `Seal(dek, aad, pt)` / `Open(dek, aad, ct)` with a version byte | Versioned ciphertext | Format can evolve (e.g. XChaCha) without a flag day | Bare GCM output |
| DD-4 | AAD = purpose ‖ column ‖ identity_id | Context binding | Blocks ciphertext cut-and-paste between rows/columns | No AAD |
| DD-5 | `DEKCache` keyed by `key_id`, TTL + LRU, eviction on erase | Cache-aside | Reads without KMS round trips; `key_id` prevents stale cross-replica reuse | Keyed by identity_id |
| DD-6 | Blind index via `transit/hmac` with domain prefix | Blind index (CipherSweet) | Searchable equality without the key in the service | Deterministic encryption (leaks more, needs the key) |
| DD-7 | Domain `pii` package owns validation, normalisation, masking | Rich domain | One masking rule set; API and tests share it | Masking in handlers / UI |
| DD-8 | Reveal = audit INSERT → COMMIT → respond | Audit-before-disclose | A reveal without a durable audit row is impossible | Log-after-response |
| DD-9 | Erase = TX(audit + DELETE subject_key cascade) → cache evict | Crypto-shredding | Single atomic destruction point | Overwrite columns with NULL only |
| DD-10 | `keys rewrap` CLI with optimistic `WHERE wrapped_dek = $old` | Online re-key | Safe with concurrent writers, resumable | Stop-the-world migration |
| DD-11 | Periodic OpenBao token + self-renew goroutine | Lease renewal | No long-lived static credential; token file replaced by init job | Root token; AppRole (more moving parts locally) |
| DD-12 | Plaintext values held in `[]byte` where practical and zeroed after sealing; DEKs zeroed on cache eviction | Best-effort memory hygiene | Go GC can copy memory, so this is defence in depth only (documented) | — |
| DD-13 | PII types implement `slog.LogValuer` / `fmt.Stringer` returning `[REDACTED]` | Redacting types | Accidental `slog.Any("info", pi)` cannot leak | Relying on reviewers |
