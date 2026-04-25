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
  - `collector_status.json` (execution outcome per collector — see R072)
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
collector_status.json, query_catalog.json, query_runs.ndjson, and
query_results.ndjson.

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

### Schema Intelligence Pack

**ARQ-SIGNALS-R057**: The system shall collect a constraint inventory
from pg_constraint including constraint type, table, column(s), and
referenced table for foreign keys. Multi-column constraints shall be
unnested with ordinal position.

**ARQ-SIGNALS-R058**: The system shall collect an index definition
inventory from pg_indexes including schema, table, index name, and
the full CREATE INDEX definition text.

**ARQ-SIGNALS-R059**: The system shall collect column-level planner
statistics from pg_stats including n_distinct, correlation, null_frac,
and avg_width. Data sample columns (most_common_vals, histogram_bounds)
shall be excluded.

**ARQ-SIGNALS-R060**: The system shall collect a column inventory from
pg_attribute with type information via format_type(). Default expression
text shall NOT be emitted (security). System columns (attnum <= 0) and
dropped columns shall be excluded.

**ARQ-SIGNALS-R061**: The system shall collect a schema namespace
inventory from pg_namespace with owner information.

**ARQ-SIGNALS-R062**: The system shall collect a view inventory from
pg_views (metadata only, no definition text).

**ARQ-SIGNALS-R063**: The system shall collect view definitions from
pg_get_viewdef in a separate collector from the inventory.

**ARQ-SIGNALS-R064**: The system shall collect a materialized view
inventory including populated and indexed status.

**ARQ-SIGNALS-R065**: The system shall collect materialized view
definitions in a separate collector from the inventory.

**ARQ-SIGNALS-R066**: The system shall collect partition topology
from pg_partitioned_table and pg_inherits including parent-child
relationships and partition bounds.

**ARQ-SIGNALS-R067**: The system shall collect a trigger inventory
from pg_trigger using tgtype bitmask encoding. Internal triggers
(tgisinternal) shall be excluded.

**ARQ-SIGNALS-R068**: The system shall collect trigger definitions
from pg_get_triggerdef in a separate collector.

**ARQ-SIGNALS-R069**: The system shall collect a function/procedure
inventory from pg_proc (PG 11+) including language, kind (function/
procedure/aggregate/window), and volatility. Function bodies shall
NOT be included in the inventory collector.

**ARQ-SIGNALS-R070**: The system shall collect function body
definitions from pg_proc.prosrc in a separate high-sensitivity
collector.

**ARQ-SIGNALS-R071**: The system shall collect a sequence inventory
from pg_sequences including data type, current value, min/max,
increment, and cycle configuration.

### Collector Execution Model

**ARQ-SIGNALS-R072**: The system shall record the execution outcome
of every registered collector for each snapshot cycle. The status
metadata (collector_status.json) shall be included in every export
ZIP alongside metadata.json. The schema is defined in
specifications/collector_status.md. Status values:
- success: query ran and returned results (or legitimate empty)
- partial: query ran with known limitations
- skipped: query was not attempted (version, extension, config)
- failed: query was attempted but produced an error

Reason categories for non-success statuses:
- version_unsupported (skipped)
- extension_missing (skipped)
- config_disabled (skipped)
- execution_error (failed)
- permission_denied (failed)
- timeout (failed)
- savepoint_rollback (failed)

**ARQ-SIGNALS-R073**: The system shall support target-scoped export.
When exporting for a specific target, query_runs, query_results, and
collector_status shall contain only data for that target. The
collector_status shall be synthesized from query_runs for target
exports.

### Deterministic Ordering

**ARQ-SIGNALS-R074**: All collector output shall be deterministically
ordered. Specifically:
- Query catalog entries: ordered by query_id
- Collector status entries: ordered by collector id
- Schema collector results: ordered by ORDER BY clauses in the
  collector SQL (typically schema name, object name)
- Export ZIP file entries: written in a fixed order (metadata,
  collector_status, snapshots, catalog, runs, results)

### Collector Sensitivity

**ARQ-SIGNALS-R075**: The system shall classify collectors that emit
application-authored SQL text — `pg_views_definitions_v1`,
`pg_matviews_definitions_v1`, `pg_triggers_definitions_v1`,
`pg_functions_definitions_v1` — as **high-sensitivity** and disable
them by default. They run only when the operator opts in via
`signals.high_sensitivity_collectors_enabled: true` (or the
`ARQ_SIGNALS_HIGH_SENSITIVITY_COLLECTORS_ENABLED=true` environment
variable). When disabled, each shall appear in
`collector_status.json` with `status=skipped` and
`reason=config_disabled`. This control exists for local operator
control over data sensitivity; it is not an exfiltration boundary
(Arq Signals runs inside the operator's own environment).

### Configuration Validation

**ARQ-SIGNALS-R076**: The system shall perform strict configuration
validation at startup, before any collection begins. Validation
distinguishes hard errors (abort with actionable message) from
warnings (log and continue). The full taxonomy is defined in
`appendix-b-configuration-schema.md` ("Validation rules"). In
particular: malformed `ARQ_SIGNALS_*` environment variable values
(e.g., non-integer for an integer field) are hard errors, not
silently dropped.

### Persistence

**ARQ-SIGNALS-R036**: The system shall persist collected data locally so that
it survives process restarts. The persistence layer shall support:
- Atomic writes (collection results stored transactionally)
- Retention-based cleanup (data older than configured days is deleted)
- Schema migration (storage schema is versioned and auto-migrated on startup)
- An instance identifier (generated on first run, stable across restarts)

**ARQ-SIGNALS-R077**: A collection cycle's query runs, query results,
and the legacy snapshot row shall be persisted atomically within a
single local-storage transaction. Partial persistence (e.g., legacy
snapshot present without query runs, or vice versa) shall not be
observable to readers or in exports.

The specific storage engine is an implementation choice, but the guarantees
above must be maintained.

### Audit logging and export metadata

**ARQ-SIGNALS-R078**: The system shall emit structured audit events
covering the operationally significant lifecycle moments — startup
configuration validation, per-target collection cycles, and export
requests — and shall extend export metadata with the fields required
to reconstruct the running posture of the daemon at the moment data
was produced. The intent is to support SOC 2 / ISO 27001 readiness:
auditors must be able to reconstruct *what* ran, *when*, *under what
configuration*, and *what was exported*, without learning *which
secrets* were involved.

Audit events are slog records carrying the structured key
`audit_event=<name>` plus typed attributes. Specifically:

- **Startup events**:
  - `audit_event=config_validated`, `status=ok|error`,
    `warnings=N`, `hard_errors=N`.
  - `audit_event=high_sensitivity_collectors`, `enabled=true|false`.
  - `audit_event=targets_loaded`, `enabled=N`, `disabled=N`.
- **Collection events** (per target, per cycle):
  - `audit_event=collection_started`, `target=<name>`.
  - `audit_event=collection_completed`, `target=<name>`,
    `snapshot_id=<id>`, `status=success|partial|failed`,
    `duration_ms=N`, `collectors_total=N`, `collectors_success=N`,
    `collectors_failed=N`, `collectors_skipped=N`.
- **Export events**:
  - `audit_event=export_requested`, `source_ip=<ip>`,
    `target_id=<id-or-empty>`, `since=<value>`, `until=<value>`.
  - `audit_event=export_completed`, `status=success|failed`,
    `duration_ms=N`, `size_bytes=N`, `error_category=<short tag>`
    (only on failure).

Audit events shall **never** include passwords, API tokens, full
connection strings, query result payloads, or any other field that
could exfiltrate secrets or production data. Field names that begin
with `password`, contain `token`, or hold a DSN-like value
(`postgres://`, `host=… password=…`) are explicitly banned from
audit-event attributes.

**Export metadata** (the `metadata.json` member of every export
ZIP) shall include at minimum:

| Field | Purpose |
|-------|---------|
| `arq_signals_version` | Build version of the daemon that produced the export. |
| `schema_version` | Snapshot/export schema version. |
| `generated_at` | Timestamp the export was produced (UTC, RFC 3339). |
| `instance_id` | Stable identifier of the producing daemon instance. |
| `target_name` | Target's logical name when the export is target-scoped; absent otherwise. |
| `high_sensitivity_collectors_enabled` | Whether the high-sensitivity gate (R075) was open at collection time. |
| `collector_status_schema_version` | Version of the `collector_status.json` schema, separate from the top-level snapshot schema. |

These fields make it possible for an auditor to determine whether a
given export contains application-authored SQL definitions (R075)
without having to parse the body of the ZIP, and to align the
export against the daemon version that produced it.

### Operational metrics endpoint

**ARQ-SIGNALS-R079**: The system shall expose an optional Prometheus
`/metrics` endpoint that publishes **operational health metrics
about the Arq Signals daemon itself**. The endpoint shall **never**
expose collected PostgreSQL data, SQL text, query results, view or
function definitions, or any sensitive payload — its scope is
limited to counters, gauges, and histograms describing the daemon's
own behaviour.

The endpoint is **disabled by default**. It is enabled by setting
`signals.metrics_enabled: true` (or the equivalent
`ARQ_SIGNALS_METRICS_ENABLED=true` environment variable). The
serving path defaults to `/metrics` and may be overridden via
`signals.metrics_path` (or `ARQ_SIGNALS_METRICS_PATH`). Setting the
path to `/health` is forbidden — the unauthenticated health endpoint
is reserved for liveness probes.

When enabled, the endpoint is mounted on the same HTTP listener as
the rest of the API and inherits the existing bearer-token auth
contract (R011). This is consistent with the rest of the API surface
and gives operators a single auth surface to manage. Operators that
prefer unauthenticated scraping should bind the listener to
loopback or to an internal network and rely on network-level
controls; the daemon itself does not relax auth on a per-path basis.

The metric set shall be exactly:

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `arq_signal_collection_cycles_total` | counter | `target`, `status` | Per-target collection cycles, labelled `success` / `partial` / `failed`. |
| `arq_signal_collection_failures_total` | counter | `target`, `reason` | Per-target hard failures (`reason` ∈ `connect_error`, `safety_check`, `persistence`, `internal`). |
| `arq_signal_collection_duration_seconds` | histogram | `target`, `status` | Wall-clock duration of each cycle. |
| `arq_signal_collectors_succeeded_total` | counter | `target` | Sum of per-cycle successful collector counts. |
| `arq_signal_collectors_failed_total` | counter | `target`, `reason` | Sum of per-cycle failed collector counts; `reason` is the same enum used in `collector_status.json` (`permission_denied`, `timeout`, `execution_error`). |
| `arq_signal_collectors_skipped_total` | counter | `target`, `reason` | Sum of per-cycle skipped collector counts; `reason` ∈ `config_disabled`, `version_unsupported`, `extension_missing`. |
| `arq_signal_export_requests_total` | counter | `status` | All export requests, labelled `success` / `failed`. |
| `arq_signal_export_failures_total` | counter | `error_category` | Failed exports, keyed by the same category emitted in audit logs. |
| `arq_signal_export_duration_seconds` | histogram | `status` | Wall-clock duration of each export. |
| `arq_signal_sqlite_persistence_failures_total` | counter | (none) | Count of `InsertCollectionAtomic` failures (R077 rollbacks). |
| `arq_signal_last_successful_collection_timestamp` | gauge | `target` | Unix seconds of the most recent successful collection per target. |
| `arq_signal_high_sensitivity_collectors_enabled` | gauge | (none) | `1` if the R075 gate is open, `0` otherwise. |

Label cardinality is bounded:

- `target` ranges over operator-configured targets (a small fixed
  set per deployment).
- `status`, `reason`, and `error_category` are fixed enums whose
  values are listed in the table.

The following label values are **explicitly forbidden** because they
would create unbounded cardinality or reintroduce sensitive content:
collector / query IDs, database names, host names, user names, file
paths, raw error message bodies, SQL text.

### Version-aware query catalog

**ARQ-SIGNALS-R081**: The system shall determine the connected
target's PostgreSQL major version, installed extensions, current
database, and current user via a single discovery probe at the start
of each collection cycle, before any catalog filtering. The discovery
result drives version-specific SQL selection so collectors continue
to work as PostgreSQL evolves its `pg_stat_*` schema.

The system supports first-class catalogs for PostgreSQL major
versions **14, 15, 16, 17, and 18**. PostgreSQL 19 is treated as
**experimental**: the collector falls back to the highest supported
catalog (PG 18) and logs a warning so operators see the
experimental status. Major versions below 14 are out of scope.

Per-major catalog files (`internal/pgqueries/catalog_pg14.go`
through `catalog_pg19.go`) carry **only the SQL that differs from
the version-agnostic default**. Most collectors share one default
SQL across all supported majors and need no per-version override.
This minimises duplication and keeps the SQL diff visible per major.

The following invariants apply to any version-specific SQL override:

- **Stable logical IDs**: a collector's logical ID
  (`pg_stat_io_v1`, etc.) is the same across all majors. Consumers
  see one ID; only the SQL underneath changes. There is no
  `pg_stat_io_v1_pg18` flavour.
- **Normalized output columns**: when PostgreSQL renames or
  restructures columns between majors, each version's SQL emits the
  same canonical column set (the union of pre- and post-rename
  columns). Columns that don't have a native source on a given
  major are emitted as `NULL` of the appropriate type.
- **No new SELECT *** in version-specific overrides. Any SELECT *
  that exists is a documented R037 dynamic-capture exemption (see
  `pg_stat_statements_v1`) or a pre-existing cross-version
  compromise to be tightened in a follow-up.
- **Safety linter applies equally**: override SQL is lint-checked at
  registration time exactly like the default registry. Override SQL
  cannot weaken R002 (DDL/DML rejection) or R013 (read-only
  enforcement).

When a collector exists in the version-agnostic registry but has no
SQL that's executable on the connected major (because of removed
underlying views or unsupported syntax), the system shall emit a
`status=skipped, reason=version_unsupported` entry in
`collector_status.json` for that collector, the same way `R075`
emits `reason=config_disabled` for high-sensitivity collectors that
the operator did not opt into.

### Control-plane boundary

**ARQ-SIGNALS-R082**: The system shall operate in one of two modes
— **standalone OSS** (Mode A) and **Arq-managed** (Mode B) — and
shall preserve a clear trust boundary between locally-controlled
configuration and externally-driven orchestration.

This rule is **DESIGN-ONLY**. It defines the contract; runtime
behavior lands in phased follow-up work (see "Future implementation
plan" below). No code or operator-visible behavior changes from
R082's introduction.

#### Modes

**Mode A — Standalone OSS.** The default. Targets are configured in
`signals.yaml`; collection is driven locally on the operator's poll
interval. No external service can request collection or narrow the
target set. This is the mode every open-source user runs.

**Mode B — Arq-managed.** Optional. When enabled by local config,
Arq Signal accepts authenticated collection requests from the
commercial Arq control plane. The control plane may **narrow** the
configured target set to a subset (the contracted/licensed
databases) and trigger collection cycles. The target set in
`signals.yaml` remains the authoritative ceiling — Arq cannot
expand it.

#### Trust boundary

- Arq Signal runs inside the customer environment. Default behavior
  has no outbound data egress (R007 / R008 / R009 remain in force).
- Arq may also run inside the customer environment depending on
  the deployment topology; the design does not assume Arq is remote.
- Mode B requests are **authenticated** (Phase 1: bearer token,
  consistent with R011). Stronger auth (mTLS, signed JWTs) is a
  future extension.
- Arq Signal **does not enforce commercial licensing as a security
  boundary**. The collector is open source; an operator who removes
  the license check can do so trivially. The commercial value lives
  in Arq's analysis, recommendations, and reporting — not in
  obscured collector behavior.

#### Target selection

- The list of targets in `signals.yaml` is the **authoritative
  ceiling**. Mode B can only narrow this set, never expand it.
- Mode B requests reference targets by their configured `name`.
  Unknown names are rejected with an explicit reason; an unknown
  name in the request does not cause silent partial success.
- Targets marked `enabled: false` are not collected even when Mode
  B explicitly requests them. Future spec extensions may relax this
  behind an explicit `allow_disabled` flag; R082 does not.

#### Proposed API shape

```
POST /collect/now
Authorization: Bearer <token>
Content-Type: application/json

{
  "targets": ["prod-main", "prod-reporting"],
  "reason": "scheduled_arq_cycle",
  "request_id": "01J5K6T3HW2A4DGCXV5Z6P0M3R"
}
```

| Field | Type | Required | Behaviour |
|---|---|---|---|
| `targets` | string[] | optional | Subset of configured target names. When **absent**, behaviour matches Mode A — collect all enabled targets. When **present and non-empty**, the cycle's effective set is `targets ∩ enabled-configured-targets`. An **empty array** (`"targets": []`) is treated as a client bug and rejected with `400 Bad Request`; collectors are never silently dropped. Backward compatible: empty body / no body retains existing semantics. |
| `reason` | string matching `^[A-Za-z0-9_-]{1,64}$` | optional | Short label surfaced in audit events. Restricted to the same charset as `request_id` so neither field can carry log-injection bytes or unbounded whitespace. Not free-form prose. |
| `request_id` | string matching `^[A-Za-z0-9_-]+$` (≤ 32 chars) | optional | Correlation identifier propagated through to per-target audit events. Restricted to ASCII alphanumerics, `_`, and `-` so audit-log greppability stays predictable. When absent, Arq Signal generates a ULID (which already satisfies the regex). |

Response:

```
202 Accepted
Content-Type: application/json

{
  "request_id": "01J5K…",
  "accepted_targets": ["prod-main", "prod-reporting"],
  "rejected_targets": []
}
```

When `targets` includes any name that is not present in
`signals.yaml`, or any target marked `enabled: false`, or when
`targets` is an empty array, the request returns `400 Bad Request`
with the rejected names + reason. The cycle is **not triggered**;
disabled targets are never silently dropped from the accepted set.

#### Audit requirements

`/collect/now` emits one of three top-level audit events per
request, each carrying the actor and (when supplied) the
correlation id:

| Event | When emitted | Carries |
|---|---|---|
| `collect_now_requested` | request was accepted; cycle was queued. | `actor`, `request_id`, `requested_targets`, `accepted_targets`, optional `reason`. |
| `collect_now_rejected` | request failed validation. The cycle is **not** queued. | `actor`, `error` (one of `invalid_json`, `invalid_request_id`, `invalid_reason`, `empty_targets_array`, `targets_not_collectible`), plus the same target / id / reason fields as far as they were parsed before the rejection. |
| `collect_now_dropped` | request passed validation but the on-demand channel buffer is already full (a previous on-demand request is queued; R032 prevents overlapping cycles). The cycle for **this** request_id will not run. | `actor`, `request_id`, `reason_category=previous_request_pending`. |

Successful cycles also propagate the `request_id` through to the
per-target events:

- `collection_started` — `target`, plus `request_id` when non-empty.
- `collection_completed` — `target`, `snapshot_id`, `status`,
  `duration_ms`, the four `collectors_*` counters, plus
  `request_id` when non-empty.

Field semantics:

- `request_id` — when the caller did not supply one, Arq Signal
  generates a ULID. Always present on `collect_now_requested` and
  `collect_now_dropped`. May be absent on `collect_now_rejected`
  (e.g. an invalid `request_id` field never produces a usable id).
- `requested_targets` — explicit list when the request narrowed the
  cycle, the literal string `all_enabled` when the `targets` field
  was absent. Audit attribute values are bounded; never an
  unbounded label set.
- `accepted_targets` — list of target names actually scheduled.
- `rejected_targets` — list of `{name, reason}` records on
  `collect_now_rejected` for the per-target failure paths. The
  `reason` enum: `unknown_target`, `disabled_target`.
- `reason` — the request's optional `reason` label, surfaced
  verbatim (already charset-validated).
- `actor` — `local_operator` for every Phase 1 / Phase 2 request,
  regardless of request shape. The `arq_control_plane` actor value
  is reserved for Phase 3, where a separate
  `signals.arq_control_plane_token` distinguishes the control-plane
  identity from the operator identity. **Until Phase 3 ships, the
  presence of a `request_id` does not change the actor field** —
  inferring control-plane identity from request shape would let any
  caller forge the audit log.

The R078 audit-attribute denylist remains in force. Secrets, SQL
payloads, raw request bodies, and PG row data are never present
in audit attributes regardless of request shape.

#### Security requirements

- Bearer token on every Mode B request (same token mechanism as
  R011). Stronger auth (mTLS, signed JWTs, separate operator vs
  control-plane tokens) is future work.
- The request can **only narrow** the configured target set. Any
  target name not present in `signals.yaml` is rejected.
- High-sensitivity collectors (R075) remain gated by local config.
  Mode B cannot enable them.
- Existing per-IP rate limiter for invalid auth (R024) applies
  unchanged. **No new rate limiting is introduced by R082.** The
  collector's existing serialization (R032 — `running.TryLock` in
  `runCycle`) already prevents overlapping cycles: a flood of
  accepted Mode B requests collapses into one in-flight cycle plus
  log-level "skipped — previous cycle still running" entries. A
  dedicated rate limit on accepted requests can be added in
  Phase 4+ if abuse patterns appear.

#### Licensing model

| Layer | Owner |
|---|---|
| License validation | Arq |
| Contracted target list / customer entitlement | Arq |
| Analysis, scoring, recommendations | Arq |
| Enterprise workflow + reporting | Arq |
| Local extraction | Arq Signal |
| Local API + this endpoint | Arq Signal |
| Local snapshot export | Arq Signal |
| Operational metrics (R079) | Arq Signal |
| Safe collector execution | Arq Signal |

Arq Signal does **not** validate licenses, check entitlements, or
gate collection on a commercial signal. Operators running Mode A
get the full collector capability — that is by design and matches
the BSD-3-Clause license posture (R009). The commercial value
remains in Arq's analysis layer, not in obscured collector behaviour.

#### Non-goals (R082)

- No license enforcement inside Arq Signal.
- No remote SaaS control plane assumption (Mode B works equally
  well for in-cluster Arq).
- No collector profile selection — that is a separate future spec.
- No per-collector export view — R080 remains separate.
- No status callback channel from Arq Signal back to Arq.
  Mode B is fire-and-forget on the request side; Arq retrieves
  results via the existing `/export` endpoint.
- No per-`request_id` outcome tracking. R082 propagates the
  correlation id through audit events but does not expose a way
  to look up "what happened to request X?" via the API.
  Outcome-by-request_id retrieval is Phase 4+ work.
- No new rate limiting on accepted requests beyond the existing
  collector serialization (see Security requirements above).

#### Future implementation plan

| Phase | Scope | Spec status |
|---|---|---|
| 1 | `POST /collect/now` accepts optional JSON body with `targets` field. Empty array, unknown names, or disabled names → 400 with rejected list. Backward compatible with empty-body POSTs. Audit `actor` remains `local_operator`. | Implemented from R082 directly. |
| 2 | `request_id` (regex `^[A-Za-z0-9_-]+$`, ≤32 chars) + `reason` (≤64 chars) fields. Audit-event extension with `requested_targets` / `accepted_targets` / `rejected_targets`. Correlation id propagated through per-target `collection_started` / `collection_completed` events. Audit `actor` still `local_operator`. | Implemented from R082 directly. |
| 3 | `signals.mode: standalone \| arq_managed` config flag. Separate `signals.arq_control_plane_token` so the operator can distinguish actor identity in audit events. Mode B requires the flag to be set. **First phase in which the audit `actor` field can carry `arq_control_plane`.** | Specified in R083 (see below). |
| 4 | Collector profiles, entitlement metadata exchange, per-`request_id` outcome lookup endpoint, status callback channel. Optional rate limiting on accepted Mode B requests if real-world abuse patterns appear. | Out of scope for R082 / R083. Separate spec. |

### Mode B authentication and configuration

**ARQ-SIGNALS-R083**: When the operator opts into Mode B (R082) by
setting `signals.mode: arq_managed`, the system shall accept
authenticated requests from the Arq control plane via a **separate
bearer token** distinct from the local API token. The audit `actor`
field is derived from *which token matched* — never from request
shape — so audit identity cannot be forged by a caller that holds
only the local API token.

This rule is **DESIGN-ONLY**. It defines the contract for the R082
Future-implementation-plan Phase 3 row; runtime behaviour lands in
a single follow-up implementation slice. R083 does not by itself
introduce any code or operator-visible behaviour change.

#### Config proposal

```yaml
signals:
  # R083: Mode B opt-in. "standalone" (default) keeps Phase 1 /
  # Phase 2 behaviour byte-for-byte. "arq_managed" activates the
  # arq_control_plane_token check.
  mode: standalone

  # R083: Separate bearer token for the Arq control plane.
  # Used ONLY when mode=arq_managed. Supplied via file (preferred)
  # or env var; never as a YAML literal — same posture as
  # api.token (R011).
  arq_control_plane_token_file: /etc/arq/control-plane.token
  # alternative:
  # arq_control_plane_token_env: ARQ_CONTROL_PLANE_TOKEN
```

| Field | Type | Default | Validation |
|---|---|---|---|
| `signals.mode` | enum `standalone` \| `arq_managed` | `standalone` | hard error on any other value |
| `signals.arq_control_plane_token_file` | path | empty | required when `mode: arq_managed`; file is re-read on every authentication attempt to support rotation without restart |
| `signals.arq_control_plane_token_env` | env-var name | empty | mutually exclusive with `_file` |

The token value is **never accepted as a YAML literal** — same
posture as `api.token`. R078's audit-attribute denylist keeps
the token out of any audit record.

Env-var overrides (consistent with R076 / appendix B):

- `ARQ_SIGNALS_MODE` → `signals.mode`
- `ARQ_SIGNALS_ARQ_CONTROL_PLANE_TOKEN_FILE` → file path
- `ARQ_SIGNALS_ARQ_CONTROL_PLANE_TOKEN_ENV` → name of the env var
  carrying the token (indirection mirrors `password_env`)

#### Auth behaviour

The existing bearer-token middleware (R011) is extended to
compare the supplied token to **both** configured tokens in
constant time:

```
Authorization: Bearer <token>
       │
       ├─ matches api.token                   → actor = local_operator
       ├─ matches arq_control_plane_token     → actor = arq_control_plane
       │  (only when mode=arq_managed)
       └─ matches neither                     → 401, rate limiter records failure
```

Once the actor is determined, it is attached to the request
context and surfaced on every audit event the request emits.
The actor never changes mid-request and never depends on request
body shape (R082 invariant carried forward).

In `mode=standalone`, the `arq_control_plane_token` config (if
present) is **ignored at auth time** — only `api.token` is
consulted. A request that would have matched the control-plane
token simply gets a 401, identical to any other unknown token.
This keeps the standalone deployment posture identical to
Phase 1 / Phase 2.

#### Audit behaviour

The Phase 2 actor invariant ("always `local_operator`") relaxes:

| Phase | Audit `actor` source |
|---|---|
| 1 / 2 | always `local_operator` (field exists but always carries this value) |
| 3 | `local_operator` when `api.token` matched; `arq_control_plane` when `arq_control_plane_token` matched, **and only when `mode: arq_managed`** |

Audit events whose `actor` value is now sourced from the auth
match:

- `collect_now_requested` / `collect_now_rejected` /
  `collect_now_dropped` (R082 Phase 2 — these already carry
  `actor`; only the value changes)
- `collection_started` / `collection_completed` when correlated
  by `request_id`
- `export_requested` / `export_completed` (R078) — Phase 3
  extends these to carry `actor` so an auditor can distinguish
  exports triggered by the local operator from those triggered
  by the control plane

A new startup audit event records the active mode and whether a
control-plane token is configured. The token *value* is never
logged — only its configured/not-configured boolean status:

```
audit_event=mode_configured
mode=arq_managed
arq_control_plane_token_configured=true
```

#### Backward compatibility

- `mode: standalone` is the default. A daemon with no
  `signals.mode` setting behaves byte-for-byte like Phase 2.
- `api.token` continues to authorise everything it authorises
  today, in both modes. Operators do not need to migrate.
- The 202 / 400 / 401 response contracts on `/collect/now` and
  `/export` are unchanged.
- Phase 1 / Phase 2 audit-event names and attribute schemas are
  unchanged on the wire — only the `actor` value can now carry
  `arq_control_plane` (and only in Mode B with the control-plane
  token).
- Adding `actor` to `export_requested` / `export_completed` is
  additive: existing parsers that don't read the field continue
  to work.

#### Security failure cases

R076's `ValidateStrict` gains the following hard errors. Each
aborts startup with an actionable message:

| Failure | Cause |
|---|---|
| `signals.mode is "arq_managed" but no control-plane token is configured` | Operator activated Mode B without supplying a token. |
| `signals.arq_control_plane_token is identical to api.token` | The two tokens must be distinct so `actor` is unambiguous. |
| `signals.arq_control_plane_token is shorter than 32 characters` | Same length floor as the auto-generated `api.token`. |
| `signals.arq_control_plane_token_file` does not exist or is unreadable | Symmetric with the existing `api.token_file` handling. |
| `signals.arq_control_plane_token_file` and `_env` both set | Pick one — same posture as multi-credential rejection on targets. |
| `signals.mode` is any value other than `standalone` or `arq_managed` | Typo guard. |

Runtime considerations (not startup errors):

- **Token rotation:** the file is re-read on every authentication
  attempt, so rotating the token does not require restarting the
  daemon.
- **Cross-actor confusion:** a local operator who guesses or
  steals the Arq token would see their requests audited as
  `actor=arq_control_plane`. This is acceptable — token
  compromise of either token is a separate incident class. The
  audit field reflects reality: whoever sent the request had the
  control-plane token. R024's per-IP rate limiter on invalid
  attempts continues to apply.
- **Replay protection:** out of scope for R083. The bearer token
  is the only auth surface. Higher-strength auth (mTLS, signed
  JWTs, request-bound nonces) is Phase 4+ work.
- **Network-level attacks:** Mode B does **not** require Arq to
  be remote. The recommended deployment is Arq in-cluster with
  Arq Signal, both processes on a private network. R011's
  loopback-bind guidance still applies.

#### Tests planned

| TC | Coverage |
|---|---|
| TC-SIG-081 | `signals.mode` defaults to `standalone` when unset. |
| TC-SIG-082 | `mode: arq_managed` without a configured control-plane token → startup error from `ValidateStrict`. |
| TC-SIG-083 | `arq_control_plane_token` equal to `api.token` → startup error. |
| TC-SIG-084 | `arq_control_plane_token` shorter than 32 chars → startup error. |
| TC-SIG-085 | Both `_file` and `_env` configured → startup error. |
| TC-SIG-086 | Request with valid `api.token` in any mode → 2xx with `actor=local_operator` in the corresponding audit event. |
| TC-SIG-087 | Request with valid `arq_control_plane_token` in `mode=arq_managed` → 2xx with `actor=arq_control_plane`. |
| TC-SIG-088 | Request with valid `arq_control_plane_token` in `mode=standalone` → 401 (token is ignored, treated as unknown). |
| TC-SIG-089 | Request with unknown token → 401 + rate-limiter records failure (R024 unchanged). |
| TC-SIG-090 | Token rotation: replacing the file's contents and re-issuing a request authenticates against the new value within the same process. |
| TC-SIG-091 | `mode_configured` startup audit event emitted with mode and `arq_control_plane_token_configured` boolean; token value never appears in any audit attribute. |
| TC-SIG-092 | `export_requested` / `export_completed` audit events carry the `actor` field, value derived from the matched token. |

#### Non-goals (R083)

- **No license validation in Arq Signal.** R082's licensing-model
  invariant carries forward.
- **No mTLS / signed JWTs / OIDC.** Higher-strength auth is
  Phase 4+ work.
- **No `mode: arq_managed_only` (refusing the local API token in
  Mode B).** Possible future extension; R083 keeps the local
  token usable in both modes so operators are never locked out
  by an Arq outage.
- **No per-`request_id` outcome lookup endpoint.** Phase 4+.
- **No status callback channel.** Phase 4+.
- **No collector profiles or entitlement metadata.** Separate
  spec.
- **No token-rotation API.** The file-based pattern already
  supports zero-downtime rotation.

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
- **INV-SIGNALS-10**: Collector output ordering must be deterministic.
  The same PostgreSQL state must produce byte-identical collector output.
- **INV-SIGNALS-11**: collector_status.json is always present in exports.
  It is a first-class artifact, not optional metadata.
- **INV-SIGNALS-12**: Schema intelligence collectors exclude system
  schemas (pg_catalog, information_schema, pg_toast, pg_temp_%).

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
| COVERED | 74 |
| PARTIALLY COVERED | 0 |
| UNCOVERED | 3 |

74 requirements (R001-R074) are covered by automated tests. R075-R077
were added in the 2026-04 review cycle and are tracked in
`traceability.md`; their tests land alongside the implementation
commits.
R040-R073 (diagnostic, schema intelligence, and status packs) are
covered by registration, linting, cadence, version-gating, schema
filter, output column, and deterministic ordering tests. Full
behavioral coverage of query execution requires live PostgreSQL.

## Traceability Notes

See [traceability.md](traceability.md) for the requirement-to-test mapping.

## Appendices

- [Appendix A: API Contract](appendix-a-api-contract.md)
- [Appendix B: Configuration Schema](appendix-b-configuration-schema.md)
