# Traceability Matrix: Arq Signals

## Legend

- **BEHAVIORAL**: Tests exercising actual code behavior (function calls, HTTP requests, data output)
- **STRUCTURAL**: Tests verifying code structure (source scanning, file inspection)
- **INTEGRATION**: Tests requiring live PostgreSQL (build-tag guarded)

| Rule ID | Rule Summary | Test ID(s) | Coverage Status | Evidence Type | Notes |
|---------|-------------|------------|-----------------|---------------|-------|
| R001 | PostgreSQL connection with supplied params | TC-SIG-001 | COVERED | BEHAVIORAL | Connection config validates host/port/dbname/user, application_name, default port |
| R002 | Execute only approved collector queries | TC-SIG-002, TC-SIG-003 | COVERED | BEHAVIORAL | Linter rejects DDL/DML/dangerous functions; accepts valid SELECT |
| R003 | Collect diagnostic data from PG sources | TC-SIG-004, TC-SIG-005 | COVERED | BEHAVIORAL | Catalog contains 9+ required query IDs; sorted output verified |
| R004 | Structured snapshot format (NDJSON) | TC-SIG-006, TC-SIG-007, TC-SIG-023 | COVERED | BEHAVIORAL | Encode/decode roundtrip; compression at 4096 threshold; data types preserved |
| R005 | Snapshot metadata present | TC-SIG-008 | COVERED | BEHAVIORAL | Export produces metadata.json with schema_version, collector fields, RFC3339 timestamp |
| R006 | Package snapshots into ZIP | TC-SIG-009, TC-SIG-024 | COVERED | BEHAVIORAL | ZIP contains required files; omits analyzer-only files; empty export succeeds |
| R007 | No scoring, recommendations, LLM | TC-SIG-010, TC-SIG-011, TC-SIG-012 | COVERED | STRUCTURAL | Source scan confirms no analysis/scoring/LLM modules present |
| R008 | No external AI network calls | TC-SIG-013 | COVERED | STRUCTURAL | Source scan confirms no LLM transport code |
| R009 | Suitable for OSS release | TC-SIG-014 | COVERED | STRUCTURAL | LICENSE present; no proprietary markers in source |
| R010 | Stable CLI with commands | TC-SIG-015 | COVERED | STRUCTURAL | CLI help output includes required commands |
| R011 | HTTP API endpoints (see Appendix A) | TC-SIG-016, TC-SIG-017, TC-SIG-041 | COVERED | BEHAVIORAL | /health 200 no auth; /status fields + no secret fields; /collect/now 202; /export ZIP |
| R012 | Per-query and per-target timeouts | TC-SIG-018 | COVERED | BEHAVIORAL | Default timeouts verified; timeout options override correctly |
| R013 | Three-layer read-only enforcement | TC-SIG-019 | COVERED | BEHAVIORAL + STRUCTURAL | Linter tested (10 cases); session param verified; ReadOnly tx verified |
| R014 | Version and extension filtering | TC-SIG-020 | COVERED | BEHAVIORAL | Extension exclusion/inclusion; sorted output |
| R015 | Cadence-based scheduling | TC-SIG-021 | COVERED | BEHAVIORAL | Duration constants; due-selection behavior; no catch-up |
| R016 | Credentials never cached/exported | TC-SIG-022 | COVERED | STRUCTURAL | No password column in schema; no Password field in storage types |
| R017 | Session read-only guard | TC-SIG-025 | COVERED | BEHAVIORAL + STRUCTURAL | Connection config sets read-only param; verified on acquired connection |
| R018 | Refuse superuser role | TC-SIG-026, TC-SIG-034 | COVERED | BEHAVIORAL | Superuser hard failure blocks; default is blocking |
| R019 | Refuse replication role | TC-SIG-027 | COVERED | BEHAVIORAL | Replication hard failure blocks |
| R020 | Refuse bypassrls role | TC-SIG-028 | COVERED | BEHAVIORAL | BypassRLS hard failure blocks |
| R021 | Read-only transaction enforcement | TC-SIG-025 | COVERED | BEHAVIORAL + STRUCTURAL | Dedicated connection; SET LOCAL timeouts inside transaction |
| R022 | Session timeout enforcement | TC-SIG-029 | COVERED | BEHAVIORAL + STRUCTURAL | Default timeouts; SET LOCAL verified; lock_timeout=5000 |
| R023 | Hard vs soft failure distinction | TC-SIG-030 | COVERED | BEHAVIORAL | Warnings do not block; hard failures do |
| R024 | No secrets in logs/API/export | TC-SIG-031, TC-SIG-041 | COVERED | BEHAVIORAL | DSN redaction; /status excludes secret fields |
| R025 | Actionable error messages | TC-SIG-032 | COVERED | BEHAVIORAL | Error contains remediation guidance |
| R026 | Unsafe override model | TC-SIG-033, TC-SIG-034, TC-SIG-042 | COVERED | BEHAVIORAL | Override option; default false; export metadata with bypass reasons |
| R027 | Configuration via YAML + env vars | TC-SIG-040 | COVERED | BEHAVIORAL | YAML loading, env override precedence, default values |
| R028 | Config file search order | TC-SIG-040 | COVERED | BEHAVIORAL | Defaults applied when no file found |
| R029 | Single-target container mode via env | TC-SIG-040 | COVERED | BEHAVIORAL | ARQ_SIGNALS_TARGET_* creates target with correct fields and defaults |
| R030 | Config validation at startup | TC-SIG-040 | COVERED | BEHAVIORAL | Validate catches short interval, zero retention, empty fields, multiple secrets, bad durations |
| R031 | Initial forced collection | TC-SIG-037 | COVERED | BEHAVIORAL | First cycle fires immediately; collect_error event proves execution |
| R032 | Overlap prevention | TC-SIG-038 | COVERED | BEHAVIORAL | Rapid CollectNow calls do not block (buffered channel dedup) |
| R033 | Concurrent multi-target collection | TC-SIG-039 | COVERED | BEHAVIORAL | 3 targets with maxConcurrent=2; all 3 attempted; errors per target |
| R034 | Commit failure blocks persistence | TC-SIG-036 | COVERED | STRUCTURAL | Commit error checked; return precedes downstream persistence |
| R035 | Export metadata contract | TC-SIG-042 | COVERED | BEHAVIORAL | Export metadata contains unsafe_mode and dynamic bypass reasons |
| R036 | Persistence guarantees | TC-SIG-043 | COVERED | BEHAVIORAL | Migration creates tables; instance ID stable; retention cleanup; batch insert atomic |
| R037 | Dynamic column capture for version-sensitive views | TC-SIG-044 | COVERED | BEHAVIORAL | pg_stat_statements uses SELECT *; linter accepts; NDJSON preserves all dynamic columns including future/renamed ones |
| R038 | Query failure isolation | TC-SIG-045 | COVERED | BEHAVIORAL + STRUCTURAL | Savepoint isolation in collector; ROLLBACK TO SAVEPOINT on failure; transaction not aborted |
| R039 | Dynamic capture preserves safety model | TC-SIG-046 | COVERED | BEHAVIORAL | Dynamic query passes linter; no write keywords; extension gating preserved |
| R040 | Server identity collection | TC-SIG-047 | COVERED | BEHAVIORAL | Registered, passes linter |
| R041 | Extension inventory | TC-SIG-047 | COVERED | BEHAVIORAL | Registered, passes linter |
| R042 | Checkpoint/bgwriter health | TC-SIG-047 | COVERED | BEHAVIORAL | Registered, passes linter |
| R043 | Long-running transactions | TC-SIG-047 | COVERED | BEHAVIORAL | Registered, passes linter |
| R044 | Lock-blocking chains | TC-SIG-047 | COVERED | BEHAVIORAL | Registered, passes linter |
| R045 | Login roles inventory (no password hashes) | TC-SIG-047 | COVERED | BEHAVIORAL | Registered, passes linter, no pg_authid/password access |
| R046 | Connection utilization | TC-SIG-047 | COVERED | BEHAVIORAL | Registered, passes linter |
| R047 | Planner stats staleness | TC-SIG-047 | COVERED | BEHAVIORAL | Registered, passes linter |
| R048 | pg_stat_statements reset check | TC-SIG-047 | COVERED | BEHAVIORAL | Registered, requires extension, gracefully skipped when absent |
| R049 | Replication slot risk | TC-SIG-048 | COVERED | BEHAVIORAL | Registered, no extension required, graceful empty result when no slots |
| R050 | Replication status/lag | TC-SIG-048 | COVERED | BEHAVIORAL | Registered, graceful empty result when no replicas |
| R051 | Checkpointer stats (PG 17+) | TC-SIG-048 | COVERED | BEHAVIORAL | Registered, MinPGVersion=17 verified, excluded on PG 16 |
| R052 | Vacuum health diagnostic | TC-SIG-048 | COVERED | BEHAVIORAL | Registered, includes dead_pct/xid_age/reloptions, adds value over raw stats |
| R053 | Idle-in-transaction offenders | TC-SIG-048 | COVERED | BEHAVIORAL | Registered, filters for idle-in-txn state, excludes own PID |
| R054 | Database sizes | TC-SIG-048 | COVERED | BEHAVIORAL | Registered, passes linter |
| R055 | Largest relations | TC-SIG-048 | COVERED | BEHAVIORAL | Registered, passes linter |
| R056 | Temp I/O pressure | TC-SIG-048 | COVERED | BEHAVIORAL | Registered, passes linter, no secrets |
| R057 | Schema constraint inventory | TC-SIG-050 | COVERED | BEHAVIORAL | pg_constraints_v1: registered, linter pass, 24h cadence, schema filter, unnest/ordinality multi-column, deterministic order |
| R058 | Schema index definitions | TC-SIG-050 | COVERED | BEHAVIORAL | pg_indexes_v1: registered, linter pass, 24h cadence, schema filter, indexdef included, COALESCE tablespace |
| R059 | Column planner statistics | TC-SIG-051 | COVERED | BEHAVIORAL | pg_stats_v1: registered, linter pass, 24h cadence, schema filter, no data samples, n_distinct/correlation included |
| R060 | Column inventory with types | TC-SIG-052 | COVERED | BEHAVIORAL | pg_columns_v1: registered, linter pass, 24h cadence, schema filter, pg_attribute native, format_type, no default text, excludes system/dropped columns |
| R061 | Schema namespace inventory | TC-SIG-053 | COVERED | BEHAVIORAL | pg_schemas_v1: registered, linter pass, 24h cadence, schema filter, pg_namespace + pg_roles join, nspname/nspowner/is_default |
| R062 | View inventory | TC-SIG-054 | COVERED | BEHAVIORAL | pg_views_v1: registered, linter pass, 24h cadence, schema filter, inventory mode (no definition), pg_views source |
| R063 | View definitions | TC-SIG-054 | COVERED | BEHAVIORAL | pg_views_definitions_v1: registered, linter pass, includes pg_get_viewdef definition column, separate from inventory |
| R064 | Materialized view inventory | TC-SIG-055 | COVERED | BEHAVIORAL | pg_matviews_v1: registered, linter pass, 24h cadence, schema filter, inventory mode, ispopulated + hasindexes |
| R065 | Materialized view definitions | TC-SIG-055 | COVERED | BEHAVIORAL | pg_matviews_definitions_v1: registered, linter pass, includes definition column, separate from inventory |
