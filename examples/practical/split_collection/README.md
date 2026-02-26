# Scenario: Split `orders` into hot + cold collections

Online workloads often require trimming historical data without losing queryability. This scenario shows how to stream archival documents into `orders_archive` while keeping writers online.

## Migration sequence

1. `20240301_create_orders_archive.go` – Creates the archive collection with matching schema validation and indexes. Also seeds a TTL index for soft-delete metadata.
2. `20240302_move_historic_orders.go` – Iterates over `orders` older than 18 months in 2,000-document batches, inserting copies into `orders_archive` and deleting the originals. The process stores checkpoints so it can resume if the job restarts.
3. `20240303_enforce_hot_retention_window.go` – Adds a validator to `orders` that rejects any document older than the retention window and documents the `mongosh` command for emergency overrides.

## Operational guidance

- Run migration 2 during a maintenance window with metrics watching. It voluntarily sleeps 250ms per batch to minimize lock pressure.
- For sharded clusters, pin the process to the `orders` primary to avoid cross-shard fan-out (demonstrated via `ReadPreferencePrimary` in the code).
- After migration 3, configure schedulers to move future cold data via incremental migrations or Change Streams.
