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

### TC-OC-03: Declared columns present in the payload

**Rule:** OC-R002 (normal)

**Scenario:** For each catalog/schema collector that produced rows, the
columns its spec declares are checked against the payload objects.

**Given:**
- A collected + exported snapshot ZIP; the per-collector declared
  column sets for the char-type catalog collectors.

**When:**
- Each non-empty catalog/schema payload is inspected.

**Then:**
- Every declared column for that collector is present in each payload
  object.

**Expected Result:** Pass.
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
