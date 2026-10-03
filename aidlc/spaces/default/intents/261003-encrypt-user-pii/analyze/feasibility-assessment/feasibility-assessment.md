# Feasibility Assessment — Encrypted personal information

## Verdict: **Feasible** — low technical risk, medium operational risk (key custody).

## Spike S-PII-1 (OpenBao 2.4.1 Transit, 2026-10-03)

Ran `openbao/openbao:2.4.1` in dev mode and verified with a scoped token:

| Check | Result |
| --- | --- |
| `aes256-gcm96` key; encrypt/decrypt a 32-byte DEK | ok, ciphertext `vault:v1:…` (version is embedded) |
| `hmac` key type; `transit/hmac/<key>` over a phone number | ok, deterministic `vault:v1:…` output |
| `rotate` (operator) then `rewrap` (app token) | ok, `vault:v1` → `vault:v2` without exposing the DEK |
| Policy limited to `update` on encrypt/decrypt/rewrap/hmac | app token **denied** `rotate` and `read` of the key |

## Approach check

| Concern | Assessment |
| --- | --- |
| Go crypto | stdlib `crypto/aes` + `crypto/cipher` GCM, `crypto/rand`, `crypto/hmac`; no new crypto dependency |
| OpenBao client | Hand-written HTTP adapter (same style as the Kratos/Keto adapters), ~4 endpoints |
| Latency | One Transit call per DEK unwrap; cached DEKs make reads local. Lookup = one HMAC call |
| Persistence in compose | OpenBao `file` storage on a named volume + a one-shot init/unseal job; dev-only unseal key on a volume (documented, never in prod) |
| Migration | New tables only; existing `profile` unchanged → no backfill |

## Risks

| Risk | Mitigation |
| --- | --- |
| KEK loss = all PII lost | OpenBao storage backups + unseal-key custody (prod: auto-unseal via cloud KMS); documented runbook |
| OpenBao outage | PII endpoints fail closed (503); rest of the service keeps working; DEK cache absorbs short blips |
| Dev unseal key on disk | Local only, labelled; prod uses auto-unseal and AppRole/K8s auth |
| Blind index leaks equality | Only phone is indexed; HMAC key separate and non-exportable |
