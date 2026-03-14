# Feature Specification: Arq Signals

## Purpose

Arq Signals is the open-source PostgreSQL diagnostic signal collector. It
connects to PostgreSQL instances, executes approved read-only SQL collectors,
produces structured snapshots of diagnostic data, and packages them for
transfer or storage. It contains no analysis, scoring, recommendations, or
LLM integration.

## Scope

### In Scope
- PostgreSQL connectivity and credential management
- Versioned SQL query catalog with safety linting
- Cadence-based scheduled collection
- Structured snapshot output (NDJSON + metadata)
- ZIP export for snapshot transfer
- CLI for collection operations
- HTTP API for collection and export endpoints
- SQLite local storage for collected signals
- BSD-3-Clause open-source distribution

### Out of Scope
- Requirement checking or scoring
- Derived statistics computation
- LLM integration or report generation
- Recommendations or root-cause analysis
- Proprietary analysis logic
- Dashboard (analysis views)
- Licensing / feature gating

## Inputs
- PostgreSQL connection parameters (host, port, dbname, user, credentials)
- Configuration file (`signals.yaml`) or environment variables
- Optional: target filters, cadence overrides, export time range

## Outputs
- Structured snapshots stored in local SQLite
- ZIP export packages containing:
  - `metadata.json` (collector version, timestamp, PG version, target info)
  - `query_catalog.json` (executed query definitions)
  - `query_runs.ndjson` (execution metadata per query)
  - `query_results.ndjson` (raw result payloads)

## Requirements

### Collection

**ARQ-SIGNALS-R001**: The system shall connect to a PostgreSQL instance using
supplied connection parameters (host, port, dbname, user, and one of:
password, password_file, password_env, pgpass_file).

**ARQ-SIGNALS-R002**: The system shall execute only approved collector queries.
Approval is enforced by a static linter that rejects DDL, DML, dangerous
functions, and multi-statement SQL at registration time. Unapproved queries
shall cause the process to abort at startup.

**ARQ-SIGNALS-R003**: The system shall collect diagnostic data from PostgreSQL
using at minimum the following versioned collectors:
- `pg_version_v1` — server version
- `pg_settings_v1` — runtime configuration
- `pg_stat_activity_v1` — active sessions
- `pg_stat_database_v1` — database-level statistics
- `pg_stat_user_tables_v1` — table statistics
- `pg_stat_user_indexes_v1` — index statistics
- `pg_statio_user_tables_v1` — table I/O statistics
- `pg_statio_user_indexes_v1` — index I/O statistics
- `pg_stat_statements_v1` — query statistics (when extension is installed)

Additional collectors (e.g. wraparound detection) may be registered.

**ARQ-SIGNALS-R004**: The system shall write collected outputs in a structured
snapshot format. Each query result shall be stored as NDJSON (one JSON object
per row, newline-delimited). Payloads exceeding 4096 bytes shall be
gzip-compressed.

**ARQ-SIGNALS-R005**: The system shall include snapshot metadata with each
collection run, including at minimum:
- collection timestamp (RFC3339)
- collector version (semver + commit hash)
- PostgreSQL server version
- target identifier

**ARQ-SIGNALS-R006**: The system shall package snapshots into a ZIP archive for
transfer or storage. The archive shall contain metadata.json,
query_catalog.json, query_runs.ndjson, and query_results.ndjson.

### Safety

**ARQ-SIGNALS-R007**: The system shall not perform scoring, recommendations,
root-cause analysis, or LLM interaction. No package in the Arq Signals
repository shall import or depend on modules that implement these functions.

**ARQ-SIGNALS-R008**: The system shall operate without network calls to
external AI services. No HTTP client, Unix domain socket client, or other
transport for LLM communication shall be present in the codebase.

**ARQ-SIGNALS-R009**: The system shall be suitable for open-source release
under the BSD-3-Clause license. The Arq Signals repository shall contain no proprietary
analysis logic, no proprietary prompts, no confidential content, and no credentials.

### Interface

**ARQ-SIGNALS-R010**: The system shall expose a stable CLI (`arqctl`) with at
minimum the following commands:
- `collect now` — trigger an immediate collection cycle
- `export` — download a snapshot ZIP archive
- `status` — show collector status and target connectivity
- `version` — print version information

**ARQ-SIGNALS-R011**: The system shall expose an HTTP API on a configurable
port with at minimum:
- `GET /health` — liveness probe (always 200)
- `GET /status` — collector status, target info, recent errors
- `POST /collect/now` — trigger immediate collection
- `GET /export` — download snapshot ZIP

**ARQ-SIGNALS-R012**: The system shall use per-query timeouts and a per-target
time budget to prevent slow queries from blocking collection of other targets.

**ARQ-SIGNALS-R013**: All PostgreSQL connections shall be read-only, enforced
by three layers:
1. Static linter rejecting DDL/DML at registration
2. Session-level `default_transaction_read_only=on`
3. Per-query `BEGIN ... READ ONLY` transaction

**ARQ-SIGNALS-R014**: The system shall filter eligible queries by PostgreSQL
major version and installed extensions. Queries requiring unavailable
extensions or unsupported versions shall be silently skipped.

**ARQ-SIGNALS-R015**: The system shall support cadence-based scheduling with
at minimum: 5m, 15m, 1h, 6h, 24h, and 7d intervals. Each query declares its
own cadence. The scheduler shall not catch up missed intervals.

**ARQ-SIGNALS-R016**: Credentials shall never be cached in memory beyond the
scope of a single connection attempt, never written to SQLite, and never
included in snapshot exports.

## Runtime Safety

**ARQ-SIGNALS-R017**: The system shall validate that the PostgreSQL session can
be placed into a read-only transaction posture before executing any collector
queries. If the session cannot be confirmed as read-only, collection for that
target shall fail with a clear error.

**ARQ-SIGNALS-R018**: The system shall refuse collection when the effective
PostgreSQL role has the superuser attribute (rolsuper = true).

**ARQ-SIGNALS-R019**: The system shall refuse collection when the effective
PostgreSQL role has the replication attribute (rolreplication = true).

**ARQ-SIGNALS-R020**: The system shall refuse collection when the effective
PostgreSQL role has the bypass RLS attribute (rolbypassrls = true).

**ARQ-SIGNALS-R021**: The system shall enforce read-only transaction execution
for every collector run by using BEGIN ... READ ONLY and verifying session-level
default_transaction_read_only is set to on.

**ARQ-SIGNALS-R022**: The system shall set conservative session-local timeouts
for collector execution. At minimum: statement_timeout (from configured query
timeout), lock_timeout (5 seconds), and idle_in_transaction_session_timeout
(from configured target timeout).

**ARQ-SIGNALS-R023**: The system shall distinguish between hard safety failures
(superuser, replication, bypassrls, read-only verification failure) that block
collection, and non-critical hygiene warnings (e.g. role is member of
pg_write_all_data) that are logged but do not block.

**ARQ-SIGNALS-R024**: The system shall not expose database passwords or secrets
in logs, API responses, status output, or exported snapshots. Credential
resolution errors shall be redacted.

**ARQ-SIGNALS-R025**: The system shall provide clear, actionable operator-facing
error messages when safety posture validation fails, including which check
failed and how to remediate.

**ARQ-SIGNALS-R026**: The system shall support an explicit unsafe override via
the ARQ_SIGNALS_ALLOW_UNSAFE_ROLE environment variable (default: false). When
enabled, blocked role attributes are downgraded to warnings. Unsafe mode shall
be recorded in export metadata as unsafe_mode: true with the specific bypassed
checks listed.

## Invariants

- **INV-SIGNALS-01**: Collector output is passive evidence, not interpretation.
  No collected value shall be annotated with pass/fail status, severity,
  recommendations, or scores.
- **INV-SIGNALS-02**: The SQL query catalog is the single source of truth for
  what data is collected. Adding a collector requires only a `Register()` call.
- **INV-SIGNALS-03**: The snapshot format is a stable contract. Breaking
  changes require a new version suffix (e.g. `_v2`).
- **INV-SIGNALS-04**: No proprietary prompts, scoring models, or analysis
  algorithms shall exist anywhere in the Arq Signals repository.
- **INV-SIGNALS-05**: Collection evidence must only be gathered under
  safety-constrained execution. No collector query shall execute outside a
  verified read-only session.
- **INV-SIGNALS-06**: Unsafe role posture (superuser, replication, bypassrls)
  is a hard failure, not a warning. Warnings are reserved for non-blocking
  hygiene issues.
- **INV-SIGNALS-07**: Credentials must never appear in any exported artifact,
  log line, API response, or stored record.

## Failure Conditions

- FC-01: Connection failure to PostgreSQL target → log error, skip target, continue to next
- FC-02: Query execution timeout → log warning, record error in query_run, continue
- FC-03: Linter rejects a query at registration → process aborts (panic)
- FC-04: SQLite write failure → log error, retry on next cycle
- FC-05: Export with no data → produce empty ZIP with metadata only

## Non-Goals

- Analysis or interpretation of collected data
- User management or authentication (signals runs without auth by default)
- Dashboard UI (analysis concern)
- License enforcement (open source, no gating)
- Report generation of any kind

## Coverage Summary

| Status | Count |
|--------|-------|
| COVERED | 26 |
| PARTIALLY COVERED | 0 |
| UNCOVERED | 0 |

All 26 requirements are covered by automated tests (16 original + 10 runtime safety).
requirements (R017–R026) are pending implementation.

## Traceability Notes

See [traceability.md](traceability.md) for the requirement-to-test mapping.
