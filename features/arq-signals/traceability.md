# Traceability Matrix: arq-signals

## Legend

- **BEHAVIORAL**: Tests that exercise actual code behavior (function calls, HTTP requests, data output)
- **STRUCTURAL**: Tests that verify code structure (AST scanning, source string matching, struct reflection)
- **INTEGRATION**: Tests requiring live PostgreSQL (guarded with `//go:build integration`)

| Rule ID | Rule Summary | Test ID(s) | Test File(s) | Coverage Status | Evidence Type | Notes |
|---------|-------------|------------|--------------|-----------------|---------------|-------|
| ARQ-SIGNALS-R001 | PostgreSQL connection with supplied params | TC-SIG-001 | signals_conn_test.go | COVERED | BEHAVIORAL | BuildConnConfig validates host/port/dbname/user, application_name, default port, error on empty host |
| ARQ-SIGNALS-R002 | Execute only approved collector queries | TC-SIG-002, TC-SIG-003 | signals_linter_test.go, signals_catalog_test.go | COVERED | BEHAVIORAL | 10 linter tests call LintQuery directly + catalog lint verification |
| ARQ-SIGNALS-R003 | Collect diagnostic data from PG sources | TC-SIG-004, TC-SIG-005 | signals_catalog_test.go | COVERED | BEHAVIORAL | Catalog count, required IDs, sorted output |
| ARQ-SIGNALS-R004 | Structured snapshot format (NDJSON) | TC-SIG-006, TC-SIG-007, TC-SIG-023 | signals_ndjson_test.go | COVERED | BEHAVIORAL | Encode/decode round-trip, compression threshold, data type preservation |
| ARQ-SIGNALS-R005 | Snapshot metadata present | TC-SIG-008 | signals_schema_test.go, signals_export_test.go | COVERED | BEHAVIORAL | Export produces ZIP with metadata.json containing schema_version, collector fields, RFC3339 timestamp |
| ARQ-SIGNALS-R006 | Package snapshots into ZIP | TC-SIG-009, TC-SIG-024 | signals_export_test.go | COVERED | BEHAVIORAL | WriteTo produces ZIP with required files; omits analyzer files |
| ARQ-SIGNALS-R007 | No scoring, recommendations, LLM | TC-SIG-010, TC-SIG-011, TC-SIG-012 | signals_boundary_test.go | COVERED | STRUCTURAL | Import scanning + string grep for forbidden code |
| ARQ-SIGNALS-R008 | No external AI network calls | TC-SIG-013 | signals_boundary_test.go | COVERED | STRUCTURAL | Import scanning for LLM packages |
| ARQ-SIGNALS-R009 | Suitable for OSS release | TC-SIG-014 | signals_boundary_test.go | COVERED | STRUCTURAL | LICENSE check + proprietary marker scan |
| ARQ-SIGNALS-R010 | Stable CLI with commands | TC-SIG-015 | signals_cli_test.go | COVERED | STRUCTURAL | AST scan for cobra subcommand definitions |
| ARQ-SIGNALS-R011 | HTTP API endpoints | TC-SIG-016, TC-SIG-017 | signals_api_test.go | COVERED | BEHAVIORAL | httptest: /health 200 no auth, /status auth + response fields, /collect/now 202, /export ZIP, no secret fields in /status |
| ARQ-SIGNALS-R012 | Per-query and per-target timeouts | TC-SIG-018 | signals_timeout_test.go | COVERED | BEHAVIORAL | Default timeouts (10s/60s), WithQueryTimeout/WithTargetTimeout override correctly |
| ARQ-SIGNALS-R013 | Three-layer read-only enforcement | TC-SIG-019 | signals_linter_test.go, signals_conn_test.go, signals_safety_behavioral_test.go | COVERED | BEHAVIORAL + STRUCTURAL | Linter: 10 behavioral tests. Session: BuildConnConfig param verified. Transaction: conn.BeginTx with ReadOnly verified structurally. |
| ARQ-SIGNALS-R014 | Version and extension filtering | TC-SIG-020 | signals_filter_test.go | COVERED | BEHAVIORAL | Filter function called with params, results verified |
| ARQ-SIGNALS-R015 | Cadence-based scheduling | TC-SIG-021 | signals_filter_test.go | COVERED | BEHAVIORAL | Duration constants + SelectDue behavior tested |
| ARQ-SIGNALS-R016 | Credentials never cached/exported | TC-SIG-022 | signals_credentials_test.go | COVERED | STRUCTURAL | Migration SQL has no password column, Target struct has no Password field |
| ARQ-SIGNALS-R017 | Session read-only guard | TC-SIG-025 | signals_safety_test.go, signals_safety_behavioral_test.go | COVERED | BEHAVIORAL + STRUCTURAL | BuildConnConfig sets default_transaction_read_only=on (behavioral). Read-only check uses acquired connection not pool (structural). Full validation requires live PG (integration test available). |
| ARQ-SIGNALS-R018 | Refuse superuser role | TC-SIG-026, TC-SIG-034 | signals_safety_test.go | COVERED | BEHAVIORAL | SafetyResult with superuser hard failure: IsSafe()=false, Error() contains rolsuper=true. Default collector blocks. |
| ARQ-SIGNALS-R019 | Refuse replication role | TC-SIG-027 | signals_safety_test.go | COVERED | BEHAVIORAL | SafetyResult with replication hard failure blocks |
| ARQ-SIGNALS-R020 | Refuse bypassrls role | TC-SIG-028 | signals_safety_test.go | COVERED | BEHAVIORAL | SafetyResult with bypassrls hard failure blocks |
| ARQ-SIGNALS-R021 | Read-only transaction enforcement | TC-SIG-025 | signals_safety_behavioral_test.go | COVERED | BEHAVIORAL + STRUCTURAL | Dedicated connection acquired before BeginTx (structural ordering verified). SET LOCAL used for transaction-scoped timeouts. |
| ARQ-SIGNALS-R022 | Session timeout enforcement | TC-SIG-029 | signals_safety_test.go, signals_safety_behavioral_test.go | COVERED | BEHAVIORAL + STRUCTURAL | Default timeouts verified (behavioral). SET LOCAL used inside transaction, lockTimeoutMs=5000, all three timeout params present (structural). Timeouts guaranteed on same connection via dedicated acquire. |
| ARQ-SIGNALS-R023 | Hard vs soft failure distinction | TC-SIG-030 | signals_safety_test.go | COVERED | BEHAVIORAL | SafetyResult: warnings do not block, hard failures block, mixed preserves blocking |
| ARQ-SIGNALS-R024 | No secrets in logs/API/export | TC-SIG-031 | signals_safety_test.go, signals_safety_behavioral_test.go, signals_api_test.go | COVERED | BEHAVIORAL + STRUCTURAL | RedactDSN behavioral tests. /status response verified to exclude secret_type/secret_ref (behavioral httptest). Source scan confirms no secret fields in handler (structural). |
| ARQ-SIGNALS-R025 | Actionable error messages | TC-SIG-032 | signals_safety_test.go | COVERED | BEHAVIORAL | SafetyResult.Error() contains CREATE ROLE, GRANT pg_monitor, specific attribute names |
| ARQ-SIGNALS-R026 | Unsafe override model | TC-SIG-033, TC-SIG-034 | signals_safety_test.go, signals_safety_behavioral_test.go | COVERED | BEHAVIORAL | WithAllowUnsafeRole option (behavioral). Default false (behavioral). Config field + env var parsing (behavioral). Export metadata captures dynamic bypass reasons with specific role attributes (behavioral). |
