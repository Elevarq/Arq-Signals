# Acceptance Tests: Collector Output-Contract Verification

## Feature

`specifications/collector-output-contract.md`

## Test Cases

### TC-OC-01: FK constraint serializes contype as the string "f" (the #312 lock)

**Rule:** OC-R003 (normal — the #312 regression lock)

**Scenario:** A target carrying an unindexed foreign key is collected
and exported; the `pg_constraints_v1` payload is inspected.

**Given:**
- A live target with a parent table (PK) and a child table whose FK to
  the parent is unindexed.

**When:**
- The full collection runs against the target and a snapshot ZIP is
  exported via `export.Builder.WriteTo`; `query_results.ndjson` is read
  back.

**Then:**
- The `pg_constraints_v1` payload contains a row with
  `contype == "f"` — the single-character string, not the integer
  `102`.

**Expected Result:** Pass on `::text`-cast SQL; RED on pre-#313 code.
(`TestIntegration_CollectorOutputContractAgainstRealPG`)

---

### TC-OC-02: Internal-"char" columns decode as single-char strings

**Rule:** OC-R003 (boundary)

**Scenario:** Every internal-`"char"` column across the catalog/schema
collectors is inspected in the exported payload.

**Given:**
- A collected + exported snapshot ZIP for the seeded target.

**When:**
- Each `query_results` payload for a catalog/schema collector is
  scanned for the columns `contype`, `relkind`, `relpersistence`,
  `provolatile`, `prokind`.

**Then:**
- Every such value present is a single-character JSON string, never a
  JSON number.

**Expected Result:** Pass.
(`TestIntegration_CollectorOutputContractAgainstRealPG`)

---

### TC-OC-03: Declared columns present in the payload (ALL collectors)

**Rule:** OC-R002 (normal)

**Scenario:** For **each registered collector** (#316 — not only the
catalog/schema ones) that produced rows, the columns its spec declares
are checked against the payload objects. The declared column set is
derived from the spec at test time (`## Output columns` table[s]), never
hand-copied.

**Given:**
- A collected + exported snapshot ZIP; the per-collector declared
  column sets derived from `specifications/collectors/<id>.md`
  (or the parent spec for definition-mode / MCV family variants).

**When:**
- Each non-empty payload is inspected.

**Then:**
- Every spec-declared column for that collector is present in each
  payload object. Column-dynamic collectors (`SELECT *`,
  version-variant, scalar) are checked for row presence only.

**Expected Result:** Pass; RED if a declared column is dropped or
renamed while the spec keeps it (verified by a mutation spot-check on
`pg_indexes_v1.indexname`).
(`TestIntegration_CollectorOutputContractAgainstRealPG`)

---

### TC-OC-04: Status↔payload invariant holds through the export

**Rule:** OC-R004 (boundary — the #312 status/payload invariant)

**Scenario:** Every successful run in the exported ZIP is joined to its
payload.

**Given:**
- A collected + exported snapshot ZIP.

**When:**
- `query_runs.ndjson` and `query_results.ndjson` are read; every row
  with `status == "success"` and `row_count == N` is joined by
  `run_id`.

**Then:**
- Exactly one payload exists per successful run, and its `payload`
  array holds exactly `N` objects.

**Expected Result:** Pass.
(`TestIntegration_CollectorOutputContractAgainstRealPG`)

---

### TC-OC-06: Boundary normalization fixes the columns #313's per-column casts missed

**Rule:** OC-R005 (normal — the #319 central-fix regression lock)

**Scenario:** A target is seeded so the columns the per-column `::text`
sweep missed are exercised: a table with a non-PK index (so
`catalog_bloat_v1`/`catalog_index_bloat_v1` emit `relkind`), a function
(so `catalog_schema` emits `provolatile`/`volatility`), and an identity
column (so `catalog_schema` emits `attidentity`). The exported payloads
are inspected.

**Given:**
- A live target carrying a heap table with a secondary index, a SQL
  function, and a `GENERATED ... AS IDENTITY` column.

**When:**
- The full collection runs and a snapshot ZIP is exported via
  `export.Builder.WriteTo`; `query_results.ndjson` is read back.

**Then:**
- Every `relkind` value in `catalog_bloat_v1`/`catalog_index_bloat_v1`
  is a single-character string (e.g. `"r"`, `"i"`), never a number.
- Every `provolatile`/`volatility` value in the function payload is a
  single-character string (e.g. `"i"`, `"s"`, `"v"`), never a number.
- Every `attidentity` value in the identity/column payload is a string
  (`""` for the non-set case, `"a"`/`"d"` when set), never a number.

**Expected Result:** RED with the OID-18 `AfterConnect` registration
removed; GREEN with it in place.
(`TestIntegration_CollectorOutputContractAgainstRealPG`)

---

### TC-OC-07: Remaining schema collectors decode their aliased char columns as strings

**Rule:** OC-R006 (normal — the #326 schema-family regression lock)

**Scenario:** A target is seeded so the schema collectors #314 missed
emit rows: a partitioned table (so `pg_partitions_v1` emits
`partition_strategy`), a user trigger (so `pg_triggers_v1` /
`pg_triggers_definitions_v1` emit `tg_enabled`), and a SQL function (so
`pg_functions_v1` / `pg_functions_definitions_v1` emit `volatility`).
The exported payloads are inspected.

**Given:**
- A live target carrying a `PARTITION BY RANGE` table with a child
  partition, a `BEFORE INSERT` row trigger, and a SQL function.

**When:**
- The full collection runs and a snapshot ZIP is exported via
  `export.Builder.WriteTo`; `query_results.ndjson` is read back.

**Then:**
- `pg_partitions_v1.partition_strategy` is a single-character string
  (e.g. `"r"` for RANGE), never a number.
- `pg_triggers_v1.tg_enabled` and `pg_triggers_definitions_v1.tg_enabled`
  are single-character strings (e.g. `"O"`), never a number.
- `pg_functions_v1.volatility` and `pg_functions_definitions_v1.volatility`
  are single-character strings (e.g. `"i"`), never a number.
- Each of these collectors emits at least one such value (the sweep is
  not vacuously satisfied).

**Expected Result:** RED with the OID-18 `AfterConnect` registration
removed; GREEN with it in place.
(`TestIntegration_CollectorOutputContractAgainstRealPG`)

---

### TC-OC-08: FDW foreign-table relkind is a string (capability-gated)

**Rule:** OC-R007 (normal / capability-gated — the #326 FDW leg)

**Scenario:** When the environment provides the FDW capability (a
superuser DSN provisions `postgres_fdw` + a foreign server + grants
USAGE to the collector role), a foreign table is seeded so
`fdw_foreign_tables_v1` emits a row; otherwise the FDW fixture and
assertion are skipped with a documented reason.

**Given:**
- `SIGNALS_TEST_PG_SUPERUSER_DSN` present and the `postgres_fdw`
  extension available (CI provisions both).

**When:**
- The full collection runs and a snapshot ZIP is exported; the
  `fdw_foreign_tables_v1` payload is read back.

**Then:**
- `fdw_foreign_tables_v1.relkind` is the single-character string `"f"`,
  never a number.

**And (capability absent):**
- With no superuser DSN / no `postgres_fdw`, the FDW fixture is not
  created and the assertion is skipped with a logged reason; every other
  assertion still runs.

**Expected Result:** Pass when the capability is present; documented
skip otherwise.
(`TestIntegration_CollectorOutputContractAgainstRealPG`)

---

### TC-OC-05: Skips locally, runs in the CI matrix

**Rule:** Constraints (env-gating)

**Scenario:** The harness is invoked without a target DSN.

**Given:**
- `SIGNALS_TEST_PG_DSN` is unset (local developer machine).

**When:**
- `go test ./...` runs (the `integration` build tag is not selected).

**Then:**
- The test is not compiled into the default suite and does not fail;
  under `-tags integration` with no DSN it SKIPS. In CI with the PG
  service container it RUNS across PG 14/15/16/17/18.

**Expected Result:** Skip locally, run in CI.
(`TestIntegration_CollectorOutputContractAgainstRealPG`)

---

### TC-OC-09: Every collector is accounted for (no silent coverage gap)

**Rule:** OC-R008 (normal / boundary — the #316 completeness gate)

**Scenario:** After collection + export, every registered collector is
classified: rows-asserted, column-dynamic row-presence, or zero-row
allowlisted with a reason. A collector emitting no rows that is not
allowlisted fails.

**Given:**
- A collected + exported snapshot ZIP and the full registry
  (`pgqueries.All()`).

**When:**
- Each registered collector is looked up in the exported payloads.

**Then:**
- A collector with a non-empty payload has its declared columns
  asserted (or its row presence, when column-dynamic).
- A collector with no rows is present in the zero-row allowlist with a
  documented reason; otherwise the harness fails naming the
  unclassified collector(s).
- The harness logs a coverage report: the asserted count and the
  enumerated not-exercised allowlist.

**Expected Result:** Pass when every collector is classified; RED with a
descriptive failure if a new collector emits no rows and is not
allowlisted.
(`TestIntegration_CollectorOutputContractAgainstRealPG`)

---

### TC-OC-10: Non-"char" type classes land in the Analyzer-expected JSON form

**Rule:** OC-R009 (normal — the #320 type-fidelity regression lock)

**Scenario:** A target is seeded (enum + composite type, extended
statistics, RLS-enabled child, populated tables, and — when the
capability is present — an FDW server) so a representative collector
column of each audited PostgreSQL type class emits a non-null value. The
exported ZIP is read back through the same `encoding/json` decode a
consumer uses and each class is asserted to be its expected JSON shape.

**Given:**
- A collected + exported snapshot ZIP for the seeded target.

**When:**
- The audited representative columns are inspected in
  `query_results.ndjson`:
  - numeric: `bloat_estimate_v1.bloat_ratio`,
    `index_bloat_estimate_v1.bloat_ratio`,
    `connection_utilization_v1.pct_used`,
    `planner_stats_staleness_v1.estimate_drift_pct`,
    `vacuum_health_v1.dead_pct`;
  - jsonb: `pg_stat_activity_summary_v1.by_backend_type` /
    `by_wait_event_type`, `pg_role_capabilities_v1.role_attrs`;
  - array: `pg_types_v1.enum_labels` / `composite_columns`,
    `index_health_summary_v1.column_set`,
    `pg_statistic_ext_v1.attnums` / `kinds`;
  - FDW option object (array rendered to object by the redactor):
    `fdw_wrappers_v1.fdw_options`, `fdw_servers_v1.server_options`,
    `fdw_foreign_tables_v1.foreign_table_options` (capability-gated);
  - timestamp: `cluster_identity_v1.postmaster_start_time`,
    `server_identity_v1.started_at`, `pg_stat_wal_v1.stats_reset`,
    `vacuum_health_v1.last_analyze`, `pg_stat_user_tables_v1.last_analyze`;
  - oid: `pg_class_storage_v1.relid`, `pg_attribute_storage_v1.atttypid`,
    `index_health_summary_v1.index_oid`, `database_sizes_v1.datid`,
    `bloat_estimate_v1.table_oid`;
  - bool: `pg_class_storage_v1.relhasindex` / `relispartition`,
    `index_health_summary_v1.is_unique`,
    `pg_functions_v1.security_definer`,
    `pg_role_capabilities_v1.is_superuser`.

**Then:**
- numeric columns are JSON numbers (a `NaN` numeric may be the string
  `"NaN"`); jsonb columns are JSON objects; array columns are JSON
  arrays; FDW option columns are JSON objects; timestamp columns are
  RFC3339 strings parseable by `time.Parse`; oid columns are JSON
  numbers; bool columns are JSON `true`/`false`.
- Each asserted column emits at least one non-null value (no vacuous
  pass).

**Expected Result:** Pass on the current codecs; RED if any class
regresses to the wrong JSON shape (mutation-verified: registering a
`pgtype` codec that returns `numeric` as a string, or asserting bool as
`"t"`/`"f"`, turns the relevant leg RED).
(`TestIntegration_CollectorTypeContractAgainstRealPG`)

---

### TC-OC-11: Every audited type class is accounted for (no silent type gap)

**Rule:** OC-R010 (normal / boundary — the #320 type-class coverage map)

**Scenario:** After collection + export, every audited type class is
classified: asserted (≥1 seeded non-null combination) or not-exercised
with a reason.

**Given:**
- A collected + exported snapshot ZIP and the audited type-class map.

**When:**
- Each audited type class (numeric, jsonb, array, FDW-option-object,
  timestamp, oid, bool, bytea) is looked up.

**Then:**
- Each class with a collector column has ≥1 asserted combination.
- `bytea` is recorded not-exercised (no collector emits a `bytea`
  column); version/capability-gated columns absent from this
  environment are covered by the OC-R008 zero-row accounting and never
  produce a false failure.
- The harness logs a per-class coverage report (asserted combinations
  and not-exercised reasons).

**Expected Result:** Pass when every class is classified; RED if an
audited class has zero asserted combinations and no not-exercised
reason.
(`TestIntegration_CollectorTypeContractAgainstRealPG`)
