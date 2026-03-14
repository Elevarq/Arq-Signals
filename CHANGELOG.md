# Changelog

All notable changes to arq-signals will be documented in this file.

This project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-03-14

### Added

- Initial open-source release of arq-signals
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
