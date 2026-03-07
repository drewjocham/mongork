# Scenario: Add `preferred_locale` Without Downtime

Goal: introduce `customers.profile.preferred_locale` while keeping legacy `users.locale` reads/writes working until client services dual-write.

## Migration sequence

1. `20240201_add_shadow_locale_field.go` – expands the schema (adds validator + default value) and seeds a small number of pilot docs.
2. `20240202_backfill_shadow_locale.go` – replays all existing users in batches of 1,000 documents, storing checkpoints after each chunk so it can resume after crashes.
3. `20240203_remove_legacy_locale.go` – contracts the schema by removing the `users.locale` field and enforcing the validator on the new field.

## Rollout checklist

1. Ship application code that **dual writes** to `users.locale` and `customers.profile.preferred_locale`.
2. Run migrations 1 and 2.
3. After verifying readers consume the new field, stop writing to `users.locale` and run migration 3.

The Go implementations showcase:

- Chunked backfill (`limit+sort+lastID` pattern)
- Idempotent updates (`$set` only when needed)
- Resume-ability using the `migration_progress` collection
