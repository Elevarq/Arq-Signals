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
- Local persistent storage for collected signals
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
- Structured snapshots stored in local persistent storage
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
root-cause analysis, or LLM interaction. No module in the Arq Signals
codebase shall depend on components that implement these functions.

**ARQ-SIGNALS-R008**: The system shall operate without network calls to
external AI services. No transport for LLM communication shall be present
in the codebase.

**ARQ-SIGNALS-R009**: The system shall be suitable for open-source release
under the BSD-3-Clause license. The repository shall contain no proprietary
analysis logic, no proprietary prompts, no confidential content, and no
credentials.

### Interface

**ARQ-SIGNALS-R010**: The system shall expose a stable CLI with at minimum
the following commands:
- `collect now` — trigger an immediate collection cycle
- `export` — download a snapshot ZIP archive (with optional output path)
- `status` — show collector status and target connectivity
- `version` — print version information

The CLI shall communicate with the running collector via its HTTP API. The
API address and authentication token shall be configurable via flags or
environment variables.

**ARQ-SIGNALS-R011**: The system shall expose an HTTP API on a configurable
address with the endpoints, response schemas, and authentication requirements
defined in Appendix A (API Contract).

**ARQ-SIGNALS-R012**: The system shall use per-query timeouts and a per-target
time budget to prevent slow queries from blocking collection of other targets.
The effective timeout for any single query is the minimum of: the query's own
timeout, the configured query timeout, and the remaining target time budget.

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
scope of a single connection attempt, never written to persistent storage,
and never included in snapshot exports.

### Runtime Safety

**ARQ-SIGNALS-R017**: The system shall validate that the PostgreSQL session
can be placed into a read-only transaction posture before executing any
collector queries. If the session cannot be confirmed as read-only,
collection for that target shall fail with a clear error.

**ARQ-SIGNALS-R018**: The system shall refuse collection when the effective
PostgreSQL role has the superuser attribute (rolsuper = true).

**ARQ-SIGNALS-R019**: The system shall refuse collection when the effective
PostgreSQL role has the replication attribute (rolreplication = true).

**ARQ-SIGNALS-R020**: The system shall refuse collection when the effective
PostgreSQL role has the bypass RLS attribute (rolbypassrls = true).

**ARQ-SIGNALS-R021**: The system shall enforce read-only transaction execution
for every collector run by using BEGIN ... READ ONLY and verifying
session-level default_transaction_read_only is set to on.

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
resolution errors shall be redacted before logging or returning to callers.

**ARQ-SIGNALS-R025**: The system shall provide clear, actionable operator-facing
error messages when safety posture validation fails, including which check
failed and how to remediate.

**ARQ-SIGNALS-R026**: The system shall support an explicit unsafe override via
the ARQ_SIGNALS_ALLOW_UNSAFE_ROLE environment variable (default: false). When
enabled, blocked role attributes are downgraded to warnings. Unsafe mode shall
be recorded in export metadata as unsafe_mode: true with the specific bypassed
checks listed.

### Configuration

**ARQ-SIGNALS-R027**: The system shall support configuration via a YAML file
and/or environment variables, with the schema defined in Appendix B
(Configuration Schema). Environment variables shall take precedence over
file-based values.

**ARQ-SIGNALS-R028**: The system shall search for configuration files in
order: explicit path via CLI flag, then system path `/etc/arq/signals.yaml`,
then local path `./signals.yaml`. The first file found is used.

**ARQ-SIGNALS-R029**: The system shall support configuring a single
PostgreSQL target entirely via environment variables (ARQ_SIGNALS_TARGET_*)
for containerized deployments. See Appendix B for the full variable list.

**ARQ-SIGNALS-R030**: The system shall validate configuration at startup and
reject invalid values (e.g. unparseable durations, empty required fields).
Non-blocking warnings (e.g. weak TLS mode) shall be logged without aborting.

### Collection Cycle Semantics

**ARQ-SIGNALS-R031**: The system shall run collection cycles at a configurable
interval (default: 5 minutes). The first cycle after startup shall force
execution of all eligible queries regardless of cadence, to establish a
baseline.

**ARQ-SIGNALS-R032**: The system shall prevent overlapping collection cycles.
If a cycle is still running when the next interval triggers, the new cycle
shall be skipped with a warning.

**ARQ-SIGNALS-R033**: The system shall collect from multiple targets
concurrently with a configurable maximum parallelism (default: 4). A failure
on one target shall not block or delay collection from other targets.

### Data Integrity

**ARQ-SIGNALS-R034**: If the PostgreSQL read-only transaction fails to commit
after queries have been executed, the system shall not persist query results
or record the collection as successful. The transaction commit result must be
checked and a commit failure must abort the success path for that target.

### Export Metadata

**ARQ-SIGNALS-R035**: The export metadata.json shall contain at minimum the
fields defined in Appendix A, section "Export metadata schema." When unsafe
mode is active, the metadata shall include `unsafe_mode: true` and
`unsafe_reasons` listing the specific bypassed checks.

### Version-Sensitive Collectors

**ARQ-SIGNALS-R037**: For diagnostic views whose schema varies across
PostgreSQL or extension versions (such as pg_stat_statements), the
collector shall capture the complete returned row shape dynamically.
Each row shall be serialized using the actual column names returned by
PostgreSQL at runtime. The collector shall not depend on a fixed column
list or fixed column positions for these views.

**ARQ-SIGNALS-R038**: If a version-sensitive collector query fails (e.g.
due to a missing or renamed column), the failure shall be isolated to
that query. Other collector queries in the same collection cycle shall
continue executing and producing results.

**ARQ-SIGNALS-R039**: Dynamic capture shall not weaken the read-only
safety model, credential handling guarantees, or export format
conventions.

### Diagnostic Pack 1

**ARQ-SIGNALS-R040**: The system shall collect server identity information
including PostgreSQL version number, server uptime, connected database name,
and database size.

**ARQ-SIGNALS-R041**: The system shall collect an inventory of installed
PostgreSQL extensions with their version information.

**ARQ-SIGNALS-R042**: The system shall collect checkpoint and background
writer health statistics from pg_stat_bgwriter.

**ARQ-SIGNALS-R043**: The system shall collect long-running transactions
(older than a configurable threshold) including PID, user, age, and a
truncated query snippet. Query text shall be truncated to prevent
capturing large query bodies.

**ARQ-SIGNALS-R044**: The system shall collect active lock-blocking chains
showing which sessions are blocking other sessions, including wait
durations.

**ARQ-SIGNALS-R045**: The system shall collect an inventory of login-capable
roles with their privilege flags (superuser, createdb, createrole,
replication, bypassrls). The collector shall NOT access password hashes
or the pg_authid table.

**ARQ-SIGNALS-R046**: The system shall collect connection utilization
metrics including total, active, idle, and idle-in-transaction counts
relative to max_connections.

**ARQ-SIGNALS-R047**: The system shall collect planner statistics staleness
indicators including estimated vs actual row counts, modifications since
last analyze, and estimate drift percentage.

**ARQ-SIGNALS-R048**: When pg_stat_statements is installed and
pg_stat_statements_info is available (PG 14+), the system shall collect
the statistics reset timestamp. When pg_stat_statements or
pg_stat_statements_info is unavailable, this collector shall be
gracefully skipped.

### Server Survival Pack

**ARQ-SIGNALS-R049**: The system shall collect replication slot status
including retained WAL size and active/inactive state. When no
replication slots are configured, the collector shall return an empty
result without error.

**ARQ-SIGNALS-R050**: The system shall collect replication status
including connected replicas, lag indicators, and sync state. When no
replicas are connected, the collector shall return an empty result
without error.

**ARQ-SIGNALS-R051**: On PostgreSQL 17 and later, the system shall
collect checkpoint statistics from pg_stat_checkpointer. On earlier
versions, this collector shall be gracefully skipped.

**ARQ-SIGNALS-R052**: The system shall collect a high-signal vacuum
health diagnostic that includes dead tuple percentage, XID freeze age,
autovacuum configuration overrides, and vacuum/analyze recency. This
collector adds operator-oriented context beyond raw table statistics.

**ARQ-SIGNALS-R053**: The system shall collect an actionable list of
backends in idle-in-transaction state, including PID, user, application,
transaction age, and a truncated query snippet.

**ARQ-SIGNALS-R054**: The system shall collect all database sizes for
growth monitoring and disk-risk triage.

**ARQ-SIGNALS-R055**: The system shall collect the largest user relations
by disk size to support storage triage.

**ARQ-SIGNALS-R056**: The system shall collect per-database temporary
file and byte usage to detect work_mem exhaustion pressure.

### Persistence

**ARQ-SIGNALS-R036**: The system shall persist collected data locally so that
it survives process restarts. The persistence layer shall support:
- Atomic writes (collection results stored transactionally)
- Retention-based cleanup (data older than configured days is deleted)
- Schema migration (storage schema is versioned and auto-migrated on startup)
- An instance identifier (generated on first run, stable across restarts)

The specific storage engine is an implementation choice, but the guarantees
above must be maintained.

## Invariants

- **INV-SIGNALS-01**: Collector output is passive evidence, not interpretation.
  No collected value shall be annotated with pass/fail status, severity,
  recommendations, or scores.
- **INV-SIGNALS-02**: The SQL query catalog is the single source of truth for
  what data is collected. Adding a collector requires only registering it
  with the catalog at startup.
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
- **INV-SIGNALS-08**: Collection cycles must not overlap. If the previous
  cycle is still running, the next trigger is skipped.
- **INV-SIGNALS-09**: Transaction commit failure must prevent downstream
  persistence of results and success-path recording.

## Failure Conditions

- FC-01: Connection failure to PostgreSQL target → log error, skip target,
  continue to next
- FC-02: Query execution timeout → log warning, record error in query_run,
  continue to next query
- FC-03: Linter rejects a query at registration → process aborts
- FC-04: Persistence write failure → log error, retry on next cycle
- FC-05: Export with no data → produce empty ZIP with metadata only
- FC-06: Transaction commit failure → abort success path for that target,
  do not persist results
- FC-07: Role safety check failure → block collection for that target with
  actionable error (unless unsafe override is active)

## Non-Goals

- Analysis or interpretation of collected data
- User management or authentication beyond API token
- Dashboard UI (analysis concern)
- License enforcement (open source, no gating)
- Report generation of any kind

## Coverage Summary

| Status | Count |
|--------|-------|
| COVERED | 39 |
| PARTIALLY COVERED | 0 |
| UNCOVERED | 17 |

39 of 56 requirements are covered by automated tests. R040-R056
(diagnostic packs 1 and 2) are covered by registration, linting,
version-gating, and safety tests; full behavioral coverage of query
execution requires live PostgreSQL.

## Traceability Notes

See [traceability.md](traceability.md) for the requirement-to-test mapping.

## Appendices

- [Appendix A: API Contract](appendix-a-api-contract.md)
- [Appendix B: Configuration Schema](appendix-b-configuration-schema.md)
