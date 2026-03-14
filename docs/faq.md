# Frequently Asked Questions

## Does Arq Signals send my data anywhere?

No. All data stays on your machine. Arq Signals makes no outbound connections except to your PostgreSQL databases. There is no telemetry, analytics, or phone-home functionality.

## Does Arq Signals use AI?

No. Arq Signals is a data collector. It does not contain any AI, machine learning, or language model functionality. It runs SQL queries and stores the results.

## Does Arq Signals require internet access?

Only to reach your PostgreSQL targets. If your databases are on the same network, Arq Signals works fully offline. It does not download updates, sync to a cloud, or contact external services.

## What data does Arq Signals collect?

Statistics from PostgreSQL's built-in views: `pg_stat_activity`, `pg_stat_database`, `pg_stat_user_tables`, `pg_settings`, `pg_stat_statements` (if available), and wraparound detection queries. All queries are read-only and visible in the source code.

## Can I review the SQL queries it runs?

Yes. Every query is defined in `internal/pgqueries/catalog.go` and `catalog_wraparound.go`. They are statically linted at startup to ensure they contain only SELECT statements with no DDL, DML, or dangerous functions.

## How does Arq Signals relate to Arq Analyzer?

Arq Signals is the open-source data collector. Arq Analyzer is a separate, commercial product that can ingest Arq Signals snapshots for automated analysis and reporting. Arq Signals is fully functional on its own -- you do not need Arq Analyzer to use it.

## Can I use Arq Signals without buying anything?

Yes. Arq Signals is free and open source under the BSD-3-Clause license. There are no paid tiers, usage limits, or feature gates.

## Can Arq Signals modify my database?

No. Connections enforce read-only mode at three independent layers. The SQL linter will abort the process at startup if any query contains write operations.

## What PostgreSQL versions are supported?

PostgreSQL 14 and later (actively supported versions). Some collectors require specific extensions (e.g., `pg_stat_statements`) which are automatically detected and skipped if unavailable.

## How do I integrate Arq Signals with my existing monitoring?

Arq Signals exports snapshots as ZIP archives with a documented JSON format (`arq-snapshot.v1`). You can feed these into any downstream tool, script, or pipeline. The format is stable and versioned.

## What happens if I connect with a superuser role?

Arq Signals will refuse to collect. The system validates the connected
role's attributes before each collection cycle and blocks collection if
the role has superuser, replication, or bypass RLS privileges. Create a
dedicated monitoring role with `pg_monitor` instead. An explicit
override (`ARQ_SIGNALS_ALLOW_UNSAFE_ROLE=true`) exists for lab/dev use
but is not recommended for production.
