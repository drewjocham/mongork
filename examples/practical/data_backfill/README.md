# Scenario: Enrich `products` with external catalog data

This scenario demonstrates how to run an idempotent, checkpointed data backfill that calls an external API and updates MongoDB in batches.

## Migration sequence

1. `20240405_create_backfill_metadata.go` – Adds a `backfill_metadata` collection used to track job state and throttle concurrency.
2. `20240406_backfill_products_from_catalog.go` – Streams product IDs in batches of 500, fetches supplemental data from a mock `CatalogClient`, and writes it back using `$set` operators so reruns are safe.
3. `20240407_enforce_catalog_fields.go` – Locks in the new schema by requiring the enriched fields in validators and indexes.

## Key techniques

- **Resume-ability**: Each batch persists the last processed `_id` plus a hash of the payload that was written.
- **Idempotency**: Updates are applied with `$set` only on fields that differ from the incoming payload.
- **Rate limiting**: The backfill sleeps between API calls to avoid overloading provider services.

The Go code uses interfaces so you can swap the `CatalogClient` implementation during tests.
