# Changelog

All notable changes to Arq Signals will be documented in this file.

This project adheres to [Semantic Versioning](https://semver.org/).

## [0.3.2] - 2026-04-25

Hardening release addressing the post-0.3.1 Codex review. Thirteen
findings closed across collection safety, export integrity, API
error handling, and supply-chain hygiene. No breaking changes.

This release focuses on correctness, safety, and audit completeness
under failure conditions.

### Fixed
- Unsupported PostgreSQL versions now fail closed: targets running a
  PG major below the supported minimum (currently 14) are rejected
  immediately after discovery and surfaced with a bounded reason
  (`version_unsupported`).
- Skipped collector status completeness: `collector_status.json` now
  reports every gated collector with its precise reason
  (`version_unsupported`, `extension_missing`, `config_disabled`)
  instead of silently omitting them. Unscoped exports also synthesise
  the file from `query_runs` rather than emitting an empty list.
- Skipped/failed runs no longer advance cadence: only `status='success'`
  rows count toward the next-due time, so transient failures and
  gating misconfigurations are retried on the next cycle instead of
  being hidden behind a full cadence window.
- Timeout setup failures now fail collection safely: a `SET LOCAL`
  failure on `statement_timeout`, `lock_timeout`, or
  `idle_in_transaction_session_timeout` aborts the cycle rather than
  warning and continuing without timeout safety.
- Export integrity hardened: a successful `query_run` whose result is
  missing or whose payload fails to decode now fails the export with
  a bounded error instead of silently omitting it.
- `/status` surfaces database read errors with HTTP 500 instead of
  returning zero counts that can mask storage failures.
- `/export` validates `since`/`until` as RFC3339 and rejects inverted
  ranges with HTTP 400 and bounded JSON error bodies.
- `retention_days <= 0` documented as cleanup disabled: warning text,
  schema appendix, and validation behaviour now agree that
  non-positive values disable cleanup.
- Savepoint error handling improved: `SAVEPOINT`,
  `ROLLBACK TO SAVEPOINT`, and `RELEASE SAVEPOINT` failures now abort
  the cycle with explicit errors instead of being discarded.
- `sslmode` validation tightened: values outside the libpq enum
  (`disable`, `allow`, `prefer`, `require`, `verify-ca`,
  `verify-full`) are rejected at startup. Production TLS still treats
  only `verify-ca` and `verify-full` as strong.
- `/collect/now` body size limit added: a 64 KiB cap via
  `http.MaxBytesReader`. Oversize requests return HTTP 413 with an
  `error=body_too_large` audit event.
- Panic recovery now returns JSON: the recovery middleware emits
  `Content-Type: application/json` so clients that branch on the
  response content type no longer break on a 500.
- gitleaks release install now checksum-verified in CI and release
  workflows: the tarball SHA-256 is checked against the upstream
  `gitleaks_<version>_checksums.txt` file before extraction.

### Tests
- 19 new tests in `tests/signals_codex_post_031_test.go` covering
  every fix above, plus an updated `TestCollectNowLargeBody` for the
  413 contract.

### Notes
- No breaking changes.
- No spec changes.
- API responses remain backward compatible; new validations only
  reject previously undefined/invalid input.
- Docker base image digest pinning remains deferred — `golang:1.25-alpine`
  and `alpine:3.21` continue to be tag-pinned. The remaining half of
  L-003 from the Codex review is scheduled for a follow-up that
  updates the Dockerfile, Trivy baseline, and release-verification
  doc together.
- R080 (per-collector export view) is not included in this release.

---

## [0.3.1] - 2026-04-25

### Added
- Target narrowing for POST /collect/now (R082 Phase 1)
- request_id and reason support with end-to-end audit trace (R082 Phase 2)
- Managed mode with dual-token authentication and actor separation (R083)
- Control-plane documentation and audit model documentation

### Fixed
- Audit completeness: no silent request loss (overlap path now emits collect_now_dropped)
- Audit metadata allowlist for curated fields (mode_configured event preserved)
- Warning on empty control-plane token after rotation

### Tests
- Adversarial tests for malformed JSON, large payloads, and concurrent requests
- Race detector coverage for concurrent POST /collect/now

### Notes
- No breaking changes
- No spec changes
- R080 (per-collector export view) not included in this release

---

## [0.3.0] - 2026-04-25

### Added

- High-sensitivity collectors are now explicit opt-in (R075)
- Strict startup configuration validation with fail-fast behavior (R076)
- Atomic collection-cycle persistence (R077)
- Structured audit logging and export metadata for compliance visibility (R078)
- Optional Prometheus `/metrics` endpoint for Arq Signal health (R079)
- Version-aware query catalog with per-major PostgreSQL support (14–18, 19 placeholder) (R081)
- Multi-arch container release (linux/amd64 + linux/arm64) with SBOM, provenance, and cosign signing
- Release verification documentation

### Fixed

- PostgreSQL 18 compatibility issues in `pg_stat_io_v1` and `pg_stat_wal_v1`
- Concurrency issue in ULID generation during multi-target collection
- Connection string construction safety improvements
- Production-readiness fixes from April 2026 code review

---

## [0.2.1] - 2026-03-23

### Security

- Upgrade Go from 1.24 to 1.25 (resolves CVE-2026-25679 HIGH,
  CVE-2026-27142, CVE-2026-27139)
- Upgrade runtime base image from Alpine 3.20 to 3.21
- Add container securityContext: readOnlyRootFilesystem, drop ALL
  capabilities, disallow privilege escalation
- Add pod-level seccompProfile (RuntimeDefault) and runAsGroup
- Add Dockerfile HEALTHCHECK instruction

### Changed

- Helm chart version: 0.2.0 → 0.2.1
- Trivy config findings: 1 HIGH + 3 MEDIUM + 6 LOW → 0 HIGH + 1 MEDIUM (false positive) + 1 LOW (false positive)

---

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
