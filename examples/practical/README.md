# Practical Migration Scenarios

These scenarios demonstrate zero-downtime techniques for common MongoDB changes. Each folder contains:

1. A short README describing the business context and rollout checklist.
2. One or more migrations that implement an expand/contract sequence.
3. Utilities for batching, resume-ability, and guardrails (e.g., safety switches, chunk sizes).

## Included Scenarios

- `add_column_expand_contract`: Adds `preferred_locale` to `customers.profile`, backfills batches of 1,000 documents, then removes the legacy `users.locale` field.
- `rename_field`: Renames `account_status` to `lifecycle_status` using dual writes and staged cleanup.
- `split_collection`: Moves cold `orders` data older than 18 months into `orders_archive` while keeping writers online.
- `data_backfill`: Demonstrates idempotent, checkpointed enrichment of a `products` collection sourced from an external API.

Each scenario is written as Go migrations so they can be imported directly into an app. When running the CLI with `--with-examples`, these migrations are registered alongside the simpler `examples/examplemigrations`.
