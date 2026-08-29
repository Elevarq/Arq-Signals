# wal_archiving_v1 — Collector Specification

## Purpose

WAL-archiving and backup-readiness signals so downstream analysis can
reason about recovery posture: whether continuous archiving is enabled
and healthy, whether the archiver is falling behind or failing, and
whether the WAL level supports the intended recovery/replication model.
These signals are not collected elsewhere; standby/replication lag is
covered separately by the replication collectors.

## Catalog source

- `pg_stat_archiver` (single-row, cluster-wide)
- `pg_settings` via `current_setting()` for `archive_mode`,
  `archive_timeout`, `wal_level`, and the **presence** of
  `archive_command` / `restore_command`

## Query shape

Explicit column projection (no `SELECT *`). One row per cluster.
`archive_command` and `restore_command` can embed credentials, so the
query emits only a boolean **presence** flag for each
(`current_setting(name, true) <> ''`) — never the command string. The
`, true` (missing_ok) form keeps the query robust across versions where
`restore_command` may be absent.

## Output columns

| Column | Type | Description |
|---|---|---|
| archived_count | bigint | WAL files successfully archived (cumulative) |
| failed_count | bigint | Failed archive attempts (cumulative) |
| last_archived_wal | text | Name of the last successfully archived WAL file |
| last_archived_time | timestamptz | Time of the last successful archive |
| last_failed_wal | text | WAL file of the last failed archive attempt |
| last_failed_time | timestamptz | Time of the last failed archive attempt |
| stats_reset | timestamptz | When the archiver statistics were last reset |
| archive_mode | text | `archive_mode` setting (`off` / `on` / `always`) |
| archive_timeout | text | `archive_timeout` setting (forced WAL-switch interval) |
| wal_level | text | `wal_level` setting (`minimal` / `replica` / `logical`) |
| archive_command_configured | boolean | Whether `archive_command` is set (presence only; never the command string) |
| restore_command_configured | boolean | Whether `restore_command` is set (presence only; never the command string) |

## Scope filter

None. `pg_stat_archiver` is a single-row, cluster-wide view present on
every supported version, so the collector always emits exactly one row.

## Invariants

- Exactly one row per collection (cluster-wide view).
- No command strings are ever persisted — only the boolean presence
  flags. This holds regardless of the high-sensitivity setting.

## Failure Conditions

- `pg_stat_archiver` and `current_setting()` are readable by
  `pg_monitor` / `pg_read_all_stats`; no elevated privilege is required,
  so a permission error is a genuine fault, not an expected boundary.

## Configuration

- Cadence: 15m (recovery posture changes slowly).
- Retention class: medium.

## Sensitivity

Not high-sensitivity: the collector emits counters, timestamps, and
enum-valued settings, plus **presence-only** booleans for the archive/
restore commands. It runs unconditionally (nothing to redact), and the
command strings — the only credential-bearing surface — are never read
into a persisted column.

## Analyzer requirements unblocked

- Recovery-readiness / backup-health detection: archiving disabled,
  archiver failing or stalled, `wal_level` below the required level for
  the intended recovery/replication model.

## Relationship to other collectors

- Complements the replication collectors (standby/slot lag) with the
  archiving side of durability. Distinct from `checkpointer_stats_v1` /
  `bgwriter_stats_v1`, which cover in-memory buffer/checkpoint activity.
