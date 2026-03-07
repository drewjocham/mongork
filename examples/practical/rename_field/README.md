# Scenario: Rename `account_status` to `lifecycle_status`

This pattern demonstrates how to rename a frequently queried field without blocking writers.

## Migration sequence

1. `20240210_add_lifecycle_status_shadow.go` – Adds the destination field, optional validator, and ensures dual-write safe indexes.
2. `20240211_backfill_lifecycle_status.go` – Batches updates in ascending `_id` order using resume checkpoints so the process can be restarted safely.
3. `20240212_remove_legacy_account_status.go` – Drops the legacy field and tightens validation so only `lifecycle_status` remains authoritative.

## Operational guidance

1. **Dual writes**: Deploy application code that writes both `account_status` and `lifecycle_status`.
2. **Backfill**: Run migration 2 during low-traffic windows; it automatically throttles via chunk size.
3. **Cutover**: Once production metrics confirm readers use the new field, disable writes to `account_status` and run migration 3.

## Included techniques

- Foreground vs background index awareness (diff tooling highlights risk)
- Resume-ability via the shared `migration_progress` collection
- Idempotent updates: each batch checks if the destination already matches, so reruns are safe
