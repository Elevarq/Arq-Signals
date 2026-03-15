# Collector Inventory

Arq Signals includes 29 read-only diagnostic collectors organized
into four packs. All queries execute inside `READ ONLY` transactions
with savepoint isolation. Collectors requiring unavailable extensions
or unsupported PostgreSQL versions are silently skipped.

Every query is visible in
[`internal/pgqueries/`](../internal/pgqueries/).

## Baseline collectors

Core PostgreSQL statistics from built-in views.

| Query ID | PostgreSQL source | Cadence | Notes |
|----------|-------------------|---------|-------|
| `pg_version_v1` | `version()` | 6h | Server version string |
| `pg_settings_v1` | `pg_settings` | 6h | All runtime parameters |
| `pg_stat_activity_v1` | `pg_stat_activity` | 5m | Active sessions |
| `pg_stat_database_v1` | `pg_stat_database` | 15m | Database-level counters |
| `pg_stat_user_tables_v1` | `pg_stat_user_tables` | 15m | Table scan/tuple stats |
| `pg_stat_user_indexes_v1` | `pg_stat_user_indexes` | 15m | Index usage stats |
| `pg_statio_user_tables_v1` | `pg_statio_user_tables` | 15m | Table I/O stats |
| `pg_statio_user_indexes_v1` | `pg_statio_user_indexes` | 15m | Index I/O stats |
| `pg_stat_statements_v1` | `pg_stat_statements` | 15m | Query statistics (requires extension, dynamic columns) |

## Wraparound risk collectors

Transaction ID and multixact age monitoring.

| Query ID | PostgreSQL source | Cadence | Notes |
|----------|-------------------|---------|-------|
| `wraparound_db_level_v1` | `pg_database` | 24h | Transaction ID age by database |
| `wraparound_rel_level_v1` | `pg_class` | 24h | Transaction ID age by table (top 200) |
| `wraparound_blockers_v1` | `pg_stat_activity` | 5m | Long-running transactions blocking wraparound |

## Diagnostic Pack 1

Operational health, security, and planner diagnostics.

| Query ID | PostgreSQL source | Cadence | Notes |
|----------|-------------------|---------|-------|
| `server_identity_v1` | `version()`, uptime, db size | 6h | Server version, uptime, database context |
| `extension_inventory_v1` | `pg_available_extensions` | 6h | Installed extensions with versions |
| `bgwriter_stats_v1` | `pg_stat_bgwriter` | 15m | Checkpoint and background writer health |
| `long_running_txns_v1` | `pg_stat_activity` | 5m | Transactions older than 5 minutes |
| `blocking_locks_v1` | `pg_stat_activity` | 5m | Lock-blocking chains with wait durations |
| `login_roles_v1` | `pg_roles` | 6h | Login roles with privilege flags (no password hashes) |
| `connection_utilization_v1` | `pg_stat_activity` | 5m | Connection counts vs max_connections |
| `planner_stats_staleness_v1` | `pg_stat_user_tables` + `pg_class` | 1h | Estimate drift and modifications since analyze |
| `pgss_reset_check_v1` | `pg_stat_statements_info` | 1h | Statistics reset timestamp (requires extension, PG 14+) |

## Server Survival Pack

Collectors focused on conditions that can severely degrade or bring
down a PostgreSQL server.

| Query ID | PostgreSQL source | Cadence | Notes |
|----------|-------------------|---------|-------|
| `replication_slots_risk_v1` | `pg_replication_slots` | 5m | Stale/inactive slots, retained WAL (empty when no slots) |
| `replication_status_v1` | `pg_stat_replication` | 5m | Replica lag and sync state (empty when standalone) |
| `checkpointer_stats_v1` | `pg_stat_checkpointer` | 15m | Checkpoint stats (PG 17+ only, complements bgwriter) |
| `vacuum_health_v1` | `pg_stat_user_tables` + `pg_class` | 15m | Dead tuple pressure, overdue vacuum, XID freeze age |
| `idle_in_txn_offenders_v1` | `pg_stat_activity` | 5m | Idle-in-transaction backends holding locks |
| `database_sizes_v1` | `pg_database` | 1h | All database sizes for growth monitoring |
| `largest_relations_v1` | `pg_stat_user_tables` | 1h | Top 30 relations by disk size |
| `temp_io_pressure_v1` | `pg_stat_database` | 15m | Per-database temp file usage |

## Version and extension behavior

- Collectors with `MinPGVersion` are excluded on older PostgreSQL
  versions (e.g. `checkpointer_stats_v1` requires PG 17+)
- Collectors with `RequiresExtension` are excluded when the extension
  is not installed (e.g. `pg_stat_statements_v1`)
- `pg_stat_statements_v1` uses `SELECT *` for cross-version
  compatibility — column names may differ between PG versions
- Replication collectors return empty results on standalone instances
