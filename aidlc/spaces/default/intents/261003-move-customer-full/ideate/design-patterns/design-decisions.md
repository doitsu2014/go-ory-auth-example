# Design Decisions

| # | Decision | Why |
| --- | --- | --- |
| DD1 | `pii.Field` `name`, column `name_ct`; plaintext encoding = JSON `{"first","last"}`; AAD binds the field name `name`, exactly like the other columns (which bind `phone_number`, not `phone_ct`) | Reuse the sealed-column pattern; no new crypto |
| DD2 | `pii.Name` type with redacting `String/Format/LogValue/MarshalJSON` | Same leak protection as other PII types |
| DD3 | Mask = first rune of each non-empty part + `***` | Fixed width per part |
| DD4 | Migration uses a dedicated `SetCustomerPIIName` query (insert row if none; else update `name_ct` only when `key_id` equals the subject's current key) | Adds the name without decrypting or rewriting other columns |
| DD5 | Strip from Kratos with read-compare-remove: re-read, compare `traits.name` with the encrypted value, then JSON Patch `remove`; retry once on 400/409 | *Revised in Develop:* Kratos v26.2.0 answers 400 "unsupported operation: test". Not atomic; the read-to-patch window only matters for an operator edit, since the schema no longer accepts a name |
| DD6 | Erasure ledger respected: identities with `customer.pii.erased` are only stripped | Earlier erase requests win (C4) |
| DD7 | Customer `name` in `/v1/me` and admin Customer deprecated, never populated | Additive v1 rule (C2) |
