# Code Generation Notes

| Unit | Agent | Result |
| --- | --- | --- |
| N0 contract | lead | Redocly: valid, 10 warnings that existed before |
| N1 Go + Kratos schema + dev/seed/smoke | developer | unit 189 pass, integration 223 pass, `./dev smoke` 39/39; live migration done (all customers have only the `email` trait; 21 encrypted names) |
| N2 admin web | developer | lint, typecheck and build pass; 114 tests (was 107) |
| N3 mobile | developer | analyze 0 issues; 118 pass, 1 skipped (integration, needs the backend) |
| N4 docs | lead | 08 (§8.10, §8.11), 04, 06 §6.7, API guide, ADR-0011 amendment, README |

## Deviations (accepted by lead)

1. **DD5:** read-compare-remove instead of JSON Patch `test` + `remove`, because Kratos v26.2.0 rejects `test`. Docs and DD5 are updated.
2. **AAD label:** `name` (the field name), the same as the other columns. The NAME-FR-03 wording is corrected.
3. **Empty name object:** `traits.name: {}` is stripped only, since the new schema rejects it.
4. **Report counts:** `scanned` counts only identities that still have a name trait.
5. **Seed:** no longer prints names or phone numbers. Smoke retries a Kratos 5xx on registration (the HIBP timeout flake).

## Live migration history

- **Run 1** used `test`: 21 copies committed, every strip failed, nothing was stripped early.
- **Run 2:** `stripped_only=32`. This was the crash-between-steps recovery path, exercised for real.
- **Re-runs:** all counts 0.
