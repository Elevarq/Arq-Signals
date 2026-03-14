# Acceptance Tests: arq-signals

## TC-SIG-001: PostgreSQL Connection

**Linked Rules:** ARQ-SIGNALS-R001
**Scenario:** Connect to a PostgreSQL instance with valid parameters
**Inputs:** host, port, dbname, user, password_file
**Expected Behavior:** Connection succeeds; `SELECT 1` returns without error
**Failure Expectation:** Invalid credentials → connection refused, error logged

---

## TC-SIG-002: Approved Query Enforcement

**Linked Rules:** ARQ-SIGNALS-R002
**Scenario:** Register a valid SELECT query and a dangerous INSERT query
**Inputs:** QueryDef with `SELECT ...` SQL; QueryDef with `INSERT ...` SQL
**Expected Behavior:** SELECT query registers successfully; INSERT query
causes a panic at registration time
**Notes:** Validates the linter rejects DDL/DML

---

## TC-SIG-003: Linter Rejects Dangerous Functions

**Linked Rules:** ARQ-SIGNALS-R002, ARQ-SIGNALS-R013
**Scenario:** Register queries containing pg_terminate_backend, pg_sleep, etc.
**Inputs:** QueryDefs with dangerous function calls
**Expected Behavior:** Each registration panics with a linter error

---

## TC-SIG-004: Collector Executes Registered Queries

**Linked Rules:** ARQ-SIGNALS-R003
**Scenario:** Run a collection cycle against a PostgreSQL target
**Inputs:** PostgreSQL target with pg_stat_statements extension installed
**Expected Behavior:** All 9+ registered queries execute; query_runs table
has one row per query; query_results has NDJSON payloads

---

## TC-SIG-005: Minimum Query Catalog Coverage

**Linked Rules:** ARQ-SIGNALS-R003
**Scenario:** Verify the query catalog contains all required collectors
**Inputs:** None (introspect registry)
**Expected Behavior:** `pgqueries.All()` returns at least 9 entries with IDs
matching: pg_version_v1, pg_settings_v1, pg_stat_activity_v1,
pg_stat_database_v1, pg_stat_user_tables_v1, pg_stat_user_indexes_v1,
pg_statio_user_tables_v1, pg_statio_user_indexes_v1, pg_stat_statements_v1

---

## TC-SIG-006: NDJSON Encoding

**Linked Rules:** ARQ-SIGNALS-R004
**Scenario:** Encode query results as NDJSON
**Inputs:** []map[string]any with 3 rows
**Expected Behavior:** Output is newline-delimited JSON; each line is a valid
JSON object; compressed flag is false for small payloads

---

## TC-SIG-007: NDJSON Compression

**Linked Rules:** ARQ-SIGNALS-R004
**Scenario:** Encode a large payload exceeding 4096 bytes
**Inputs:** []map[string]any with 100+ rows totaling >4KB
**Expected Behavior:** compressed flag is true; decoding returns original rows

---

## TC-SIG-008: Snapshot Metadata Present

**Linked Rules:** ARQ-SIGNALS-R005
**Scenario:** After a collection cycle, inspect the stored query_run
**Inputs:** Completed collection against a PostgreSQL target
**Expected Behavior:** query_run contains: non-empty collected_at (RFC3339),
non-empty pg_version, valid target_id; export metadata.json contains
collector_version with semver format

---

## TC-SIG-009: ZIP Export Structure

**Linked Rules:** ARQ-SIGNALS-R006
**Scenario:** Generate an export ZIP after collection
**Inputs:** At least one completed collection cycle
**Expected Behavior:** ZIP contains: metadata.json, query_catalog.json,
query_runs.ndjson, query_results.ndjson. ZIP does NOT contain:
stats_snapshots, requirement_catalog, reports, environment_profiles

---

## TC-SIG-010: No Analyzer Modules Linked

**Linked Rules:** ARQ-SIGNALS-R007
**Scenario:** Inspect arq-signals Go imports
**Inputs:** All .go files in arq-signals repository
**Expected Behavior:** No file imports any of: requirements, scoring, stats,
report, llm, doctor, analyzer packages. `go vet ./...` passes.

---

## TC-SIG-011: No LLM Dependencies

**Linked Rules:** ARQ-SIGNALS-R007, ARQ-SIGNALS-R008
**Scenario:** Search codebase for LLM-related code
**Inputs:** All .go files in arq-signals repository
**Expected Behavior:** No references to: Unix domain socket LLM client,
LLM prompt construction, report generation, stub_llm, UDSClient.
No `llm` package directory exists.

---

## TC-SIG-012: No Scoring or Recommendations

**Linked Rules:** ARQ-SIGNALS-R007
**Scenario:** Search codebase for scoring/recommendation code
**Inputs:** All .go files in arq-signals repository
**Expected Behavior:** No references to: ComputeScore, grade bands,
RequirementDef, RunAll (requirements), TopRisk, CategoryBreakdown.
No `scoring` or `requirements` package directories exist.

---

## TC-SIG-013: No External AI Network Calls

**Linked Rules:** ARQ-SIGNALS-R008
**Scenario:** Inspect all network-related code
**Inputs:** All .go files in arq-signals repository
**Expected Behavior:** No HTTP client calls to AI services. No UDS socket
connections for LLM. Only network calls are: PostgreSQL connections (pgx)
and the local HTTP API server.

---

## TC-SIG-014: OSS Readiness Check

**Linked Rules:** ARQ-SIGNALS-R009
**Scenario:** Scan repository for proprietary content
**Inputs:** All files in arq-signals repository
**Expected Behavior:** BSD-3-Clause LICENSE file exists. No proprietary
prompts, scoring algorithms, or analysis logic. No credentials, API keys,
or internal endpoints. CONTRIBUTING.md and SECURITY.md exist.

---

## TC-SIG-015: CLI Commands Available

**Linked Rules:** ARQ-SIGNALS-R010
**Scenario:** Run arqctl with --help
**Inputs:** Built binary
**Expected Behavior:** Output lists: collect, export, status, version commands

---

## TC-SIG-016: Health Endpoint

**Linked Rules:** ARQ-SIGNALS-R011
**Scenario:** GET /health on running signals server
**Inputs:** Running arq-signals process
**Expected Behavior:** Returns HTTP 200 with no authentication required

---

## TC-SIG-017: Status Endpoint

**Linked Rules:** ARQ-SIGNALS-R011
**Scenario:** GET /status with valid bearer token
**Inputs:** Running arq-signals with configured target
**Expected Behavior:** Returns JSON with target info, collection state,
recent errors. Does NOT include scoring, grades, or report data.

---

## TC-SIG-018: Per-Query Timeout

**Linked Rules:** ARQ-SIGNALS-R012
**Scenario:** Query with 1s timeout against slow-responding target
**Inputs:** Query configured with 1s timeout; target that delays >2s
**Expected Behavior:** Query times out; error recorded in query_run;
collection continues with remaining queries

---

## TC-SIG-019: Three-Layer Read-Only Enforcement

**Linked Rules:** ARQ-SIGNALS-R013
**Scenario:** Attempt write operations through the collector connection
**Inputs:** PostgreSQL target with read-only role
**Expected Behavior:** All three layers prevent writes: linter rejects at
registration, session-level read-only blocks at connection, per-query
READ ONLY blocks at transaction

---

## TC-SIG-020: Version and Extension Filtering

**Linked Rules:** ARQ-SIGNALS-R014
**Scenario:** Filter queries for PG 14 without pg_stat_statements
**Inputs:** FilterParams{PGMajorVersion: 14, Extensions: []}
**Expected Behavior:** pg_stat_statements_v1 excluded (requires extension);
all other queries included (no MinPGVersion > 14)

---

## TC-SIG-021: Cadence Scheduling

**Linked Rules:** ARQ-SIGNALS-R015
**Scenario:** SelectDue with mixed cadences and varied last-run times
**Inputs:** Queries with 5m, 15m, 1h cadences; lastRun map with timestamps
**Expected Behavior:** Only queries whose cadence has elapsed since last run
are selected. No catch-up for missed intervals.

---

## TC-SIG-022: Credential Safety

**Linked Rules:** ARQ-SIGNALS-R016
**Scenario:** After collection, inspect SQLite and export ZIP
**Inputs:** Completed collection with password_file credential
**Expected Behavior:** No password values in SQLite tables. No password
values in export ZIP. Password was read from file, used for connection,
then discarded.

---

## TC-SIG-023: Snapshot Format Stability

**Linked Rules:** ARQ-SIGNALS-R004, INV-SIGNALS-03
**Scenario:** Parse snapshot output with a fixed schema contract
**Inputs:** query_results.ndjson from export
**Expected Behavior:** Each line is valid JSON. Each object has consistent
keys matching the PostgreSQL view columns. Schema matches documented contract.

---

## TC-SIG-024: Empty Collection Export

**Linked Rules:** ARQ-SIGNALS-R006, FC-05
**Scenario:** Export when no collection data exists
**Inputs:** Fresh database with no snapshots
**Expected Behavior:** ZIP is created with metadata.json only (or with
empty NDJSON files). No error is returned.

---

## TC-SIG-025: Session Read-Only Guard

**Linked Rules:** ARQ-SIGNALS-R017, ARQ-SIGNALS-R021
**Scenario:** Validate session posture before collection
**Inputs:** Connection to PostgreSQL with standard monitoring role
**Expected Behavior:** Session has default_transaction_read_only=on, transaction
opens as READ ONLY

---

## TC-SIG-026: Superuser Role Blocked

**Linked Rules:** ARQ-SIGNALS-R018
**Scenario:** Connect with a superuser role
**Inputs:** Role attributes where rolsuper=true
**Expected Behavior:** Collection fails with error "safety check failed: role
has superuser attribute"

---

## TC-SIG-027: Replication Role Blocked

**Linked Rules:** ARQ-SIGNALS-R019
**Scenario:** Connect with a replication role
**Inputs:** Role attributes where rolreplication=true
**Expected Behavior:** Collection fails with error mentioning replication
attribute

---

## TC-SIG-028: BypassRLS Role Blocked

**Linked Rules:** ARQ-SIGNALS-R020
**Scenario:** Connect with a bypassrls role
**Inputs:** Role attributes where rolbypassrls=true
**Expected Behavior:** Collection fails with error mentioning bypassrls
attribute

---

## TC-SIG-029: Session Timeouts Applied

**Linked Rules:** ARQ-SIGNALS-R022
**Scenario:** Verify session-local timeouts are set
**Inputs:** Collector with queryTimeout=10s, targetTimeout=60s
**Expected Behavior:** statement_timeout, lock_timeout,
idle_in_transaction_session_timeout are set to appropriate values

---

## TC-SIG-030: Hard vs Soft Failure Distinction

**Linked Rules:** ARQ-SIGNALS-R023
**Scenario:** Role has pg_write_all_data membership (hygiene warning) but no
superuser/replication/bypassrls
**Inputs:** Role attributes where rolsuper=false, rolreplication=false,
rolbypassrls=false, but is member of pg_write_all_data
**Expected Behavior:** Warning logged, collection proceeds

---

## TC-SIG-031: Credential Redaction

**Linked Rules:** ARQ-SIGNALS-R024
**Scenario:** Password resolution error occurs
**Inputs:** Invalid password file path
**Expected Behavior:** Error message does not contain actual password, shows
"credential resolution failed (details redacted)" or similar

---

## TC-SIG-032: Actionable Error Messages

**Linked Rules:** ARQ-SIGNALS-R025
**Scenario:** Collection fails due to superuser role
**Inputs:** Connection where rolsuper=true
**Expected Behavior:** Error message includes: which check failed, the
attribute value, and remediation guidance (e.g. "create a dedicated monitoring
role with pg_monitor")

---

## TC-SIG-033: Unsafe Override Enabled

**Linked Rules:** ARQ-SIGNALS-R026
**Scenario:** ARQ_SIGNALS_ALLOW_UNSAFE_ROLE=true with superuser connection
**Inputs:** Superuser role + override enabled
**Expected Behavior:** Warning logged, collection proceeds, metadata includes
unsafe_mode=true and lists bypassed checks

---

## TC-SIG-034: Unsafe Override Disabled by Default

**Linked Rules:** ARQ-SIGNALS-R026
**Scenario:** Superuser connection with no override set
**Inputs:** Superuser role, no ARQ_SIGNALS_ALLOW_UNSAFE_ROLE
**Expected Behavior:** Collection fails (blocked), same as TC-SIG-026

---

## TC-SIG-035: Multiple Unsafe Attributes

**Linked Rules:** ARQ-SIGNALS-R018, ARQ-SIGNALS-R019, ARQ-SIGNALS-R020
**Scenario:** Role has both superuser and replication attributes
**Inputs:** rolsuper=true, rolreplication=true
**Expected Behavior:** Error message lists all failing attributes
