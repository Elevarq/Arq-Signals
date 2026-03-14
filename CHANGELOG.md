# Changelog

All notable changes to Arq Signals will be documented in this file.

This project adheres to [Semantic Versioning](https://semver.org/).

## [0.2.0] - 2026-03-14

### Added

- **Diagnostic Pack 1** — 9 new collectors: server identity, extension
  inventory, checkpoint/bgwriter health, long-running transactions,
  blocking locks, login role inventory, connection utilization, planner
  statistics staleness, pg_stat_statements reset check
- **Server Survival Pack** — 8 new collectors: replication slot risk,
  replication status/lag, checkpointer stats (PG 17+), vacuum health,
  idle-in-transaction offenders, database sizes, largest relations,
  temp I/O pressure
- Dynamic pg_stat_statements capture (SELECT *) for cross-version
  compatibility across PostgreSQL 14–18
- Savepoint-based query isolation — single query failure no longer
  aborts the entire collection transaction
- STDD specification expanded to 56 requirements
- Smoke-tested against PostgreSQL 14, 15, 16, 17, and 18

### Fixed

- NULL payload on zero-row query results (EncodeNDJSON nil guard)
- pg_stat_bgwriter column compatibility on PG 17+ (uses SELECT *)
- Planner stats staleness query round() type compatibility
- Transaction commit error handling (commit failure now blocks
  downstream persistence)

### Changed

- Total collectors: 12 → 29
- Total tests: 94 → 135
- Total STDD requirements: 26 → 56
- Replication-related collectors return empty results gracefully
  when replication is not configured
- pg_stat_checkpointer collector skipped on PG < 17 via version filter

---

## [0.1.0] - 2026-03-14

### Added

- Initial open-source release of Arq Signals
- PostgreSQL diagnostic signal collector with 12 versioned SQL collectors
- Read-only enforcement at three independent layers (static linter, session-level, per-query transaction)
- Cadence-based scheduling (5m, 15m, 1h, 6h, 24h, 7d)
- Automatic PostgreSQL version and extension detection
- Query filtering by PostgreSQL version and available extensions
- NDJSON result format with automatic gzip compression for payloads exceeding 4KB
- Snapshot export as ZIP archive (`arq-snapshot.v1` format)
- SQLite local storage with WAL mode
- HTTP API with bearer token authentication (health, status, collect, export)
- CLI tool (`arqctl`) for collection and export operations
- Concurrent multi-target collection with configurable worker pool
- Per-query and per-target timeout budgets
- Credential management via file, environment variable, or pgpass (never cached or exported)
- Docker support with non-root runtime (UID 10001, Alpine 3.20)
- BSD-3-Clause license

### Security

- Three-layer read-only enforcement prevents any write operations
- Static SQL linter rejects DDL, DML, and dangerous functions at startup
- Credentials are never stored in SQLite or included in exports
- No outbound network connections except to configured PostgreSQL targets
- API binds to loopback by default
