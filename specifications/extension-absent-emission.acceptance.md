# Acceptance Tests: extension-absent-emission

## Feature

`specifications/extension-absent-emission.md` — sentinel emission
for extension-gated collectors.

## Test Cases

### TC-EXTABS-01: Missing extension emits sentinel row

**Rule:** EA-R001, EA-R002

**Scenario:** An extension-gated collector runs against a target
that does not have the required extension installed.

**Given:**
- A collector with `RequiresExtension: pg_stat_statements`.
- Target PostgreSQL where `pg_stat_statements` is NOT in
  `pg_extension`.

**When:**
- A collection cycle runs.

**Then:**
- The collector contributes exactly one row to `query_results.ndjson`
  for this target.
- That row has: `available = false`,
  `reason = "extension_not_installed"`,
  `extension = "pg_stat_statements"`,
  `collector = "pg_stat_statements_v1"`.
- The `QueryRun.Error` field is empty.

**Expected Result:** Pass when the sentinel row is present with the
expected shape and no error is recorded.

---

### TC-EXTABS-02: Extension present yields normal rows only

**Rule:** INV-EA-01

**Scenario:** The extension IS installed; no sentinel is emitted.

**Given:**
- A collector with `RequiresExtension: pg_stat_statements`.
- Target with the extension installed and rows present.

**When:**
- A collection cycle runs.

**Then:**
- Normal result rows are present.
- No row has `available = false`.

**Expected Result:** Pass when normal rows are present and no
sentinel appears.

---

### TC-EXTABS-03: Insufficient privilege emits privilege-reason sentinel

**Rule:** FC-EA-02

**Scenario:** Extension is installed but the monitoring role cannot
read its rows (not a member of `pg_read_all_stats` or
`pg_monitor`).

**Given:**
- `pg_stat_statements` present.
- Role lacks `pg_monitor` / `pg_read_all_stats`.

**When:**
- A collection cycle runs.

**Then:**
- One sentinel row emitted with
  `available = false`, `reason = "insufficient_privilege"`.

**Expected Result:** Pass when the privilege sentinel is present
and no partial rows are emitted.

---

### TC-EXTABS-04: Extension-check failure records error + sentinel

**Rule:** FC-EA-01

**Scenario:** The `pg_extension` probe itself errors (catalog
corruption, permission denied on `pg_extension`).

**Given:**
- A target that returns an error from the extension detection
  query.

**When:**
- A collection cycle runs.

**Then:**
- A sentinel row emitted with
  `available = false`,
  `reason = "extension_check_failed"`.
- `QueryRun.Error` is populated with the underlying error string.

**Expected Result:** Pass when both the sentinel row and the
recorded error are present.

---

## Coverage Notes

Covers EA-R001, EA-R002, INV-EA-01, FC-EA-01, FC-EA-02. EA-R003
(spec cross-reference requirement) and EA-R004 (analyzer-side
recognition) are covered by traceability tests.
