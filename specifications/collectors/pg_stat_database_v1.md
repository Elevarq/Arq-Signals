# pg_stat_database_v1 — Collector Specification

## Purpose

Per-database activity counters: transactions, cache hits/misses,
temp-file usage, deadlocks. Baseline evidence for cache
effectiveness (`blks_hit` / `blks_read`), contention (`deadlocks`),
spill pressure (`temp_files`, `temp_bytes`). Acts as the fallback
I/O signal on PostgreSQL < 16 where `pg_stat_io` is unavailable.

## Catalog source

- `pg_stat_database`

## Output columns

| Column | Type | Description |
|---|---|---|
| datid | oid | Database OID |
| datname | text | Database name |
| numbackends | int | Current connected backends |
| xact_commit | bigint | Committed transactions (cumulative) |
| xact_rollback | bigint | Rolled-back transactions (cumulative) |
| blks_read | bigint | Disk blocks read (cumulative) |
| blks_hit | bigint | Blocks found in cache (cumulative) |
| tup_returned | bigint | Rows returned by queries |
| tup_fetched | bigint | Rows fetched by index scans |
| tup_inserted | bigint | Rows inserted |
| tup_updated | bigint | Rows updated |
| tup_deleted | bigint | Rows deleted |
| conflicts | bigint | Recovery conflicts |
| temp_files | bigint | Temp files created |
| temp_bytes | bigint | Temp bytes written |
| deadlocks | bigint | Deadlocks detected |
| blk_read_time | double precision | Time reading blocks, ms |
| blk_write_time | double precision | Time writing blocks, ms |
| stats_reset | timestamptz | Last reset |

## Scope filter

`WHERE datname IS NOT NULL` — excludes rows with NULL `datname`
(shared-catalog aggregate rows). Template databases are included
by `datname` but flagged via `datistemplate` downstream from
`pg_database_v1`.

## Invariants

- Deterministic ordering: `ORDER BY datname`.
- Stable output column order.
- Read-only query, passes linter.

## Failure Conditions

- FC-01: Permission denied → standard collector error path.
- FC-02: Counter decrease without `stats_reset` advance → per
  `delta-semantics.md` FC-DS-01.

## Configuration

- Category: database
- Cadence: 15m (Cadence15m)
- Retention: RetentionMedium
- Min PG version: 10
- Requires extension: none
- Semantics: cumulative (see `delta-semantics.md`)
- Enabled by default: yes

## Sensitivity

Low. Per-database aggregates.

## Analyzer requirements unblocked

- `io-cost-calibration` — fallback path for PG < 16.
- `query-concentration-risk` — temp-file / deadlock pressure as
  secondary evidence.
- Generic DB-health coverage.

## Known gap vs aspirational spec

PG 12+ adds `checksum_failures`, `checksum_last_failure`; PG 14+
adds `session_time`, `active_time`, `idle_in_transaction_time`,
`sessions`, `sessions_abandoned`, `sessions_fatal`,
`sessions_killed`. These are absent from the current SELECT list.
**Extension plan:** extend the SELECT list to include these fields
emitted as NULL on older PG majors; additive change, no consumer
break. Priority: before `io-cost-calibration` ships, because the
detector's confidence bands lean on `blk_read_time` which is
already present, but ideally cross-validate with `active_time`
(PG 14+) for session-weighted cost normalization.
