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
| R011 | HTTP API endpoints (see Appendix A) | TC-SIG-016, TC-SIG-017, TC-SIG-041 | COVERED | BEHAVIORAL | /health returns 200 no auth; /status returns expected fields; /status excludes secret fields |
| R012 | Per-query and per-target timeouts | TC-SIG-018 | COVERED | BEHAVIORAL | Default timeouts verified; timeout options override correctly |
| R013 | Three-layer read-only enforcement | TC-SIG-019 | COVERED | BEHAVIORAL + STRUCTURAL | Linter tested (10 cases); session param verified; ReadOnly tx verified |
| R014 | Version and extension filtering | TC-SIG-020 | COVERED | BEHAVIORAL | Extension exclusion/inclusion; sorted output |
| R015 | Cadence-based scheduling | TC-SIG-021 | COVERED | BEHAVIORAL | Duration constants; due-selection behavior; no catch-up |
| R016 | Credentials never cached/exported | TC-SIG-022 | COVERED | STRUCTURAL | No password column in schema; no Password field in storage types; no credentials in export |
| R017 | Session read-only guard | TC-SIG-025 | COVERED | BEHAVIORAL + STRUCTURAL | Connection config sets read-only param; verified on acquired connection |
| R018 | Refuse superuser role | TC-SIG-026, TC-SIG-034 | COVERED | BEHAVIORAL | Superuser hard failure blocks; error contains attribute detail; default is blocking |
| R019 | Refuse replication role | TC-SIG-027 | COVERED | BEHAVIORAL | Replication hard failure blocks |
| R020 | Refuse bypassrls role | TC-SIG-028 | COVERED | BEHAVIORAL | BypassRLS hard failure blocks |
| R021 | Read-only transaction enforcement | TC-SIG-025 | COVERED | BEHAVIORAL + STRUCTURAL | Dedicated connection; SET LOCAL timeouts inside transaction |
| R022 | Session timeout enforcement | TC-SIG-029 | COVERED | BEHAVIORAL + STRUCTURAL | Default timeouts; SET LOCAL verified; lock_timeout=5000; three params present |
| R023 | Hard vs soft failure distinction | TC-SIG-030 | COVERED | BEHAVIORAL | Warnings do not block; hard failures do; mixed preserves blocking |
| R024 | No secrets in logs/API/export | TC-SIG-031, TC-SIG-041 | COVERED | BEHAVIORAL | DSN redaction; error redaction; /status excludes secret fields |
| R025 | Actionable error messages | TC-SIG-032 | COVERED | BEHAVIORAL | Error contains remediation guidance and specific attribute names |
| R026 | Unsafe override model | TC-SIG-033, TC-SIG-034, TC-SIG-042 | COVERED | BEHAVIORAL | Override option; default false; env var; export metadata with bypass reasons |
| R027 | Configuration via YAML + env vars | TC-SIG-040 | UNCOVERED | — | New requirement; pending test |
| R028 | Config file search order | TC-SIG-040 | UNCOVERED | — | New requirement; pending test |
| R029 | Single-target container mode via env | TC-SIG-040 | UNCOVERED | — | New requirement; pending test |
| R030 | Config validation at startup | TC-SIG-040 | UNCOVERED | — | New requirement; pending test |
| R031 | Initial forced collection | TC-SIG-037 | UNCOVERED | — | New requirement; pending test |
| R032 | Overlap prevention | TC-SIG-038 | UNCOVERED | — | New requirement; pending test |
| R033 | Concurrent multi-target collection | TC-SIG-039 | UNCOVERED | — | New requirement; pending test |
| R034 | Commit failure blocks persistence | TC-SIG-036 | COVERED | STRUCTURAL | Commit error checked; return precedes downstream persistence |
| R035 | Export metadata contract | TC-SIG-042 | COVERED | BEHAVIORAL | Export metadata contains unsafe_mode and dynamic bypass reasons |
| R036 | Persistence guarantees | TC-SIG-043 | UNCOVERED | — | New requirement; pending test |
