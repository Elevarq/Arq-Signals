# Acceptance Tests: Arq Signals

All test cases describe observable behavior and constraints. They are
language-neutral and do not reference specific implementation constructs.

## TC-SIG-001: PostgreSQL Connection

**Linked Rules:** ARQ-SIGNALS-R001
**Scenario:** Connect to a PostgreSQL instance with valid parameters
**Inputs:** host, port, dbname, user, password_file
**Expected Behavior:** Connection succeeds; a simple query returns
without error
**Failure Expectation:** Invalid credentials → connection refused, error
logged

---

## TC-SIG-002: Approved Query Enforcement

**Linked Rules:** ARQ-SIGNALS-R002
**Scenario:** Register a valid SELECT query and a dangerous INSERT query
**Inputs:** A read-only SELECT collector; an INSERT collector
**Expected Behavior:** The SELECT collector registers successfully; the
INSERT collector is rejected and the process aborts at startup
**Notes:** Validates the static linter rejects DDL/DML

---

## TC-SIG-003: Linter Rejects Dangerous Functions

**Linked Rules:** ARQ-SIGNALS-R002, ARQ-SIGNALS-R013
**Scenario:** Register collectors containing pg_terminate_backend,
pg_sleep, etc.
**Inputs:** Collectors with dangerous function calls in their SQL
**Expected Behavior:** Each registration is rejected and the process
aborts

---

## TC-SIG-004: Collector Executes Registered Queries

**Linked Rules:** ARQ-SIGNALS-R003
**Scenario:** Run a collection cycle against a PostgreSQL target
**Inputs:** PostgreSQL target with pg_stat_statements extension installed
**Expected Behavior:** All 9+ registered collectors execute; one
execution record exists per collector; result payloads are stored as
NDJSON

---

## TC-SIG-005: Minimum Query Catalog Coverage

**Linked Rules:** ARQ-SIGNALS-R003
**Scenario:** Verify the query catalog contains all required collectors
**Inputs:** None (introspect the registered catalog)
**Expected Behavior:** The catalog contains at least 9 entries with IDs:
pg_version_v1, pg_settings_v1, pg_stat_activity_v1,
pg_stat_database_v1, pg_stat_user_tables_v1, pg_stat_user_indexes_v1,
pg_statio_user_tables_v1, pg_statio_user_indexes_v1,
pg_stat_statements_v1

---

## TC-SIG-006: NDJSON Encoding

**Linked Rules:** ARQ-SIGNALS-R004
**Scenario:** Encode query results as NDJSON
**Inputs:** 3 result rows with mixed data types
**Expected Behavior:** Output is newline-delimited JSON; each line is a
valid JSON object; compression flag is false for small payloads

---

## TC-SIG-007: NDJSON Compression

**Linked Rules:** ARQ-SIGNALS-R004
**Scenario:** Encode a large payload exceeding 4096 bytes
**Inputs:** 100+ result rows totaling >4KB
**Expected Behavior:** Compression flag is true; decoding returns the
original rows unchanged

---

## TC-SIG-008: Snapshot Metadata Present

**Linked Rules:** ARQ-SIGNALS-R005
**Scenario:** After a collection cycle, inspect the stored metadata
**Inputs:** Completed collection against a PostgreSQL target
**Expected Behavior:** Metadata contains: non-empty collected_at
(RFC3339 format), non-empty pg_version, valid target_id; export
metadata.json contains collector_version in semver format

---

## TC-SIG-009: ZIP Export Structure

**Linked Rules:** ARQ-SIGNALS-R006
**Scenario:** Generate an export ZIP after collection
**Inputs:** At least one completed collection cycle
**Expected Behavior:** ZIP contains: metadata.json, query_catalog.json,
query_runs.ndjson, query_results.ndjson. ZIP does NOT contain:
stats_snapshots, requirement_catalog, reports, environment_profiles

---

## TC-SIG-010: No Analyzer Modules Present

**Linked Rules:** ARQ-SIGNALS-R007
**Scenario:** Inspect the codebase for analysis/scoring/LLM components
**Inputs:** All source files in the repository
**Expected Behavior:** No source file depends on scoring, analysis,
requirement-checking, report generation, or LLM components

---

## TC-SIG-011: No LLM Dependencies

**Linked Rules:** ARQ-SIGNALS-R007, ARQ-SIGNALS-R008
**Scenario:** Search codebase for LLM-related code
**Inputs:** All source files in the repository
**Expected Behavior:** No references to LLM clients, prompt
construction, report generation, or model inference. No LLM-related
module or directory exists.

---

## TC-SIG-012: No Scoring or Recommendations

**Linked Rules:** ARQ-SIGNALS-R007
**Scenario:** Search codebase for scoring/recommendation code
**Inputs:** All source files in the repository
**Expected Behavior:** No references to score computation, grade bands,
requirement definitions, risk rankings, or recommendation text. No
scoring module or directory exists.

---

## TC-SIG-013: No External AI Network Calls

**Linked Rules:** ARQ-SIGNALS-R008
**Scenario:** Inspect all network-related code
**Inputs:** All source files in the repository
**Expected Behavior:** No outbound HTTP client calls to AI services. No
socket connections for LLM. Only network calls are: PostgreSQL
connections and the local HTTP API server.

---

## TC-SIG-014: OSS Readiness Check

**Linked Rules:** ARQ-SIGNALS-R009
**Scenario:** Scan repository for proprietary content
**Inputs:** All files in the repository
**Expected Behavior:** BSD-3-Clause LICENSE file exists. No proprietary
prompts, scoring algorithms, or analysis logic. No credentials, API
keys, or internal endpoints. CONTRIBUTING.md and SECURITY.md exist.

---

## TC-SIG-015: CLI Commands Available

**Linked Rules:** ARQ-SIGNALS-R010
**Scenario:** Run the CLI tool with a help flag
**Inputs:** Built binary
**Expected Behavior:** Help output lists: collect, export, status,
version commands

---

## TC-SIG-016: Health Endpoint

**Linked Rules:** ARQ-SIGNALS-R011
**Scenario:** GET /health on a running Arq Signals server
**Inputs:** Running Arq Signals process
**Expected Behavior:** Returns HTTP 200 with JSON body containing
"status" and "version" fields. No authentication required.

---

## TC-SIG-017: Status Endpoint

**Linked Rules:** ARQ-SIGNALS-R011
**Scenario:** GET /status with valid bearer token
**Inputs:** Running Arq Signals with configured target
**Expected Behavior:** Returns JSON with fields per Appendix A: API
Contract. Response includes target info and collection state. Response
does NOT include secret_type, secret_ref, passwords, or scoring data.

---

## TC-SIG-018: Per-Query Timeout

**Linked Rules:** ARQ-SIGNALS-R012
**Scenario:** Query with 1s timeout against slow-responding target
**Inputs:** Query configured with 1s timeout; target that delays >2s
**Expected Behavior:** Query times out; error recorded in execution
metadata; collection continues with remaining queries

---

## TC-SIG-019: Three-Layer Read-Only Enforcement

**Linked Rules:** ARQ-SIGNALS-R013
**Scenario:** Attempt write operations through the collector connection
**Inputs:** PostgreSQL target with read-only role
**Expected Behavior:** All three layers prevent writes: linter rejects
at registration, session-level read-only blocks at connection,
per-query READ ONLY blocks at transaction

---

## TC-SIG-020: Version and Extension Filtering

**Linked Rules:** ARQ-SIGNALS-R014
**Scenario:** Filter queries for PG 14 without pg_stat_statements
**Inputs:** PostgreSQL major version 14, no pg_stat_statements extension
**Expected Behavior:** pg_stat_statements_v1 excluded (requires
extension); all other queries included

---

## TC-SIG-021: Cadence Scheduling

**Linked Rules:** ARQ-SIGNALS-R015
**Scenario:** Schedule queries with mixed cadences and varied last-run
times
**Inputs:** Queries with 5m, 15m, 1h cadences; varied last execution
timestamps
**Expected Behavior:** Only queries whose cadence has elapsed since last
run are selected. No catch-up for missed intervals.

---

## TC-SIG-022: Credential Safety

**Linked Rules:** ARQ-SIGNALS-R016
**Scenario:** After collection, inspect persistent storage and export
**Inputs:** Completed collection with password_file credential
**Expected Behavior:** No password values in persistent storage tables.
No password values in export ZIP. Password was read from file, used for
connection, then discarded.

---

## TC-SIG-023: Snapshot Format Stability

**Linked Rules:** ARQ-SIGNALS-R004, INV-SIGNALS-03
**Scenario:** Parse snapshot output with a fixed schema contract
**Inputs:** query_results.ndjson from export
**Expected Behavior:** Each line is valid JSON. Each object has
consistent keys matching the PostgreSQL view columns.

---

## TC-SIG-024: Empty Collection Export

**Linked Rules:** ARQ-SIGNALS-R006, FC-05
**Scenario:** Export when no collection data exists
**Inputs:** Fresh system with no snapshots
**Expected Behavior:** ZIP is created with metadata.json only (or with
empty NDJSON files). No error is returned.

---

## TC-SIG-025: Session Read-Only Guard

**Linked Rules:** ARQ-SIGNALS-R017, ARQ-SIGNALS-R021
**Scenario:** Validate session posture before collection
**Inputs:** Connection to PostgreSQL with standard monitoring role
**Expected Behavior:** Session has default_transaction_read_only=on;
transaction opens as READ ONLY

---

## TC-SIG-026: Superuser Role Blocked

**Linked Rules:** ARQ-SIGNALS-R018
**Scenario:** Connect with a superuser role
**Inputs:** Role attributes where rolsuper=true
**Expected Behavior:** Collection fails with error indicating the
superuser attribute was detected

---

## TC-SIG-027: Replication Role Blocked

**Linked Rules:** ARQ-SIGNALS-R019
**Scenario:** Connect with a replication role
**Inputs:** Role attributes where rolreplication=true
**Expected Behavior:** Collection fails with error mentioning
replication attribute

---

## TC-SIG-028: BypassRLS Role Blocked

**Linked Rules:** ARQ-SIGNALS-R020
**Scenario:** Connect with a bypassrls role
**Inputs:** Role attributes where rolbypassrls=true
**Expected Behavior:** Collection fails with error mentioning bypassrls

---

## TC-SIG-029: Session Timeouts Applied

**Linked Rules:** ARQ-SIGNALS-R022
**Scenario:** Verify session-local timeouts are set
**Inputs:** Collector with queryTimeout=10s, targetTimeout=60s
**Expected Behavior:** statement_timeout, lock_timeout (5s), and
idle_in_transaction_session_timeout are set within the collection
transaction

---

## TC-SIG-030: Hard vs Soft Failure Distinction

**Linked Rules:** ARQ-SIGNALS-R023
**Scenario:** Role has pg_write_all_data membership but no
superuser/replication/bypassrls
**Inputs:** rolsuper=false, rolreplication=false, rolbypassrls=false,
but role is member of pg_write_all_data
**Expected Behavior:** Warning logged, collection proceeds

---

## TC-SIG-031: Credential Redaction

**Linked Rules:** ARQ-SIGNALS-R024
**Scenario:** Password resolution error occurs
**Inputs:** Invalid password file path
**Expected Behavior:** Error message does not contain the actual
password. Shown message is generic (e.g. "credential resolution
failed").

---

## TC-SIG-032: Actionable Error Messages

**Linked Rules:** ARQ-SIGNALS-R025
**Scenario:** Collection fails due to superuser role
**Inputs:** Connection where rolsuper=true
**Expected Behavior:** Error message includes: which check failed, the
attribute value, and remediation guidance (e.g. "create a dedicated
monitoring role")

---

## TC-SIG-033: Unsafe Override Enabled

**Linked Rules:** ARQ-SIGNALS-R026
**Scenario:** ARQ_SIGNALS_ALLOW_UNSAFE_ROLE=true with superuser
connection
**Inputs:** Superuser role + override enabled
**Expected Behavior:** Warning logged, collection proceeds, export
metadata includes unsafe_mode=true and lists the specific bypassed
checks

---

## TC-SIG-034: Unsafe Override Disabled by Default

**Linked Rules:** ARQ-SIGNALS-R026
**Scenario:** Superuser connection with no override set
**Inputs:** Superuser role, ARQ_SIGNALS_ALLOW_UNSAFE_ROLE not set
**Expected Behavior:** Collection fails (blocked), same as TC-SIG-026

---

## TC-SIG-035: Multiple Unsafe Attributes

**Linked Rules:** ARQ-SIGNALS-R018, ARQ-SIGNALS-R019, ARQ-SIGNALS-R020
**Scenario:** Role has both superuser and replication attributes
**Inputs:** rolsuper=true, rolreplication=true
**Expected Behavior:** Error message lists all failing attributes

---

## TC-SIG-036: Commit Failure Blocks Downstream Persistence

**Linked Rules:** ARQ-SIGNALS-R034
**Scenario:** PostgreSQL transaction commit fails after queries execute
**Inputs:** Collection queries succeed but transaction commit returns
an error
**Expected Behavior:** The collector returns an error for that target.
No query results, snapshots, or success events are persisted. The
collection is not recorded as successful.

---

## TC-SIG-037: Initial Forced Collection

**Linked Rules:** ARQ-SIGNALS-R031
**Scenario:** System starts for the first time
**Inputs:** Fresh system, no prior collection data
**Expected Behavior:** The first collection cycle executes all eligible
queries regardless of cadence scheduling

---

## TC-SIG-038: Overlap Prevention

**Linked Rules:** ARQ-SIGNALS-R032
**Scenario:** Collection cycle is still running when next interval fires
**Inputs:** Slow target that takes longer than poll interval
**Expected Behavior:** The overlapping cycle is skipped with a warning
log. The running cycle completes normally.

---

## TC-SIG-039: Partial Target Failure

**Linked Rules:** ARQ-SIGNALS-R033
**Scenario:** One of three configured targets is unreachable
**Inputs:** Three targets; one with invalid host
**Expected Behavior:** The unreachable target fails with a logged error.
The other two targets are collected successfully. The cycle completes.

---

## TC-SIG-040: Configuration File Loading

**Linked Rules:** ARQ-SIGNALS-R027, ARQ-SIGNALS-R028
**Scenario:** Load configuration from file and environment
**Inputs:** A signals.yaml file with poll_interval=10m; env var
ARQ_SIGNALS_POLL_INTERVAL=2m
**Expected Behavior:** The effective poll interval is 2m (env var
overrides file)

---

## TC-SIG-041: Status Endpoint Excludes Credentials

**Linked Rules:** ARQ-SIGNALS-R011, ARQ-SIGNALS-R024
**Scenario:** GET /status for a target configured with password_file
**Inputs:** Running system with password_file credential source
**Expected Behavior:** The /status response contains target host, port,
user, dbname. It does NOT contain secret_type, secret_ref, password,
or the path to the password file.

---

## TC-SIG-042: Export Metadata Contains Unsafe Reasons

**Linked Rules:** ARQ-SIGNALS-R035
**Scenario:** Export with unsafe mode active after collecting from a
superuser role
**Inputs:** ARQ_SIGNALS_ALLOW_UNSAFE_ROLE=true, superuser role
**Expected Behavior:** metadata.json contains unsafe_mode=true and
unsafe_reasons listing the specific bypassed check (e.g. "role has
superuser attribute (rolsuper=true)")

---

## TC-SIG-043: Retention Cleanup

**Linked Rules:** ARQ-SIGNALS-R036
**Scenario:** Data older than retention period exists
**Inputs:** Retention configured to 1 day; data collected 2 days ago
**Expected Behavior:** Old data is removed after a collection cycle.
Recent data is preserved.
