# Intent Statement — Encrypted personal information (PII)

## Problem

Customers need to store personal information (phone number, date of birth,
postal address, national ID number, display name) in the platform. Today the
`profile` table stores app data in plaintext, so a database dump, a backup
leak, or an over-privileged SQL session exposes it. The owner wants this data
**encrypted with an industry-standard PII-protection flow**, and wants the
local infra (PostgreSQL + Ory + the key manager) brought up with Docker Compose.

## Users

| User | Client | Needs |
| --- | --- | --- |
| Customer | Flutter mobile app | View, edit and erase their own personal information |
| Admin (supporter/admin/super_admin) | React admin web | See masked PII; admins with the right permission reveal full PII with a reason, audited; look a customer up by phone number |
| Operator | compose / CLI | Run a key manager, rotate keys, re-wrap data keys without downtime |
| Auditor / DPO | audit log | Prove who saw which customer's PII, when and why |

## Success criteria (observable)

1. A raw `SELECT` on the identity database shows only ciphertext and keyed
   hashes for every PII column; no plaintext PII appears in DB, logs, or audit.
2. Data keys are wrapped by a KEK held in a key manager (OpenBao Transit in
   compose); the service never stores the KEK.
3. A customer can erase their PII; afterwards it is cryptographically
   unrecoverable (crypto-shredding), also from backups.
4. Admin views are masked by default; reveal requires a permission plus a
   reason and writes an audit event.
5. KEK rotation + re-wrap runs online and keeps all data readable.
6. `make up` starts Postgres, Ory, OpenBao and the service; smoke checks pass.

## In scope

- Envelope encryption (per-customer DEK, AES-256-GCM with AAD) + KEK in OpenBao Transit.
- Blind index (HMAC-SHA256) for equality lookup on phone number.
- Personal-info API for customers and admins, masking, audited reveal.
- Crypto-shredding erasure, key rotation CLI, compose infra, mobile + web UI.

## Out of scope

- Encrypting Kratos's own tables (Kratos needs the login email in clear;
  covered by disk/volume encryption and least privilege — documented).
- Moving `name` traits out of Kratos (tracked as a follow-up).
- Production KMS wiring (AWS/GCP KMS) beyond the provider interface.
