# Collector Output-Contract Verification — Behavioral Specification

Spec version: 1.0
Status: ACTIVE
Issue: [Elevarq/Signals#314](https://github.com/Elevarq/Signals/issues/314)

## Purpose

Collector specs (`specifications/collectors/<id>.md`) declare an output
contract — a table of columns with PostgreSQL types. Until #314, no
test executed a collector against a real PostgreSQL and verified that
its declared output landed, correctly typed, in the exported snapshot
ZIP. The `pgqueries` tests are purely static (registration, read-only
linting, ordering); the SQL is never run. That gap is exactly why
`contype` serializing as the integer `102` instead of the string `"f"`
was invisible for the whole internal-`"char"` class (#312): the
`specification → tests → implementation` chain was missing the
`tests → artifact` link.

This spec defines the missing test layer: an integration harness that,
against a real PostgreSQL carrying representative schema, runs the full
collection, exports a snapshot ZIP via the production export path, reads
the ZIP back, and asserts the per-collector output contract. It locks
the #312 class permanently — the assertions go RED on pre-#313 code and
GREEN after.

Scope is the **catalog/schema collectors** (`Category == "schema"`),
with emphasis on the internal-`"char"` collectors PR #313 touched
(`pg_constraints_v1`, `pg_class_storage_v1`, `pg_functions_v1`,
`pg_functions_definitions_v1`, `pg_proc_config_v1`). The remaining
non-schema collectors are an explicit fast-follow (see
Non-Functional Requirements).

This spec complements `appendix-a-api-contract.md` §"Snapshot
data-integrity invariants (#312)" (INV-SNAP-STATUS-PAYLOAD, INV-CHAR-TEXT)
and §"Collector output-contract verification (#314)"
(INV-OUTPUT-CONTRACT, INV-CHAR-TEXT-VERIFIED, INV-STATUS-PAYLOAD-VERIFIED).

## Interfaces

### Inputs

- A live target PostgreSQL, reachable via `SIGNALS_TEST_PG_DSN`
  (build-tag `integration`; skipped when unset). In CI the target is a
  `services: postgres:` container matrixed across PG 14/15/16/17/18.
- Representative schema seeded into the target before collection:
  a parent table with a primary key, a child table whose foreign key to
  the parent is **unindexed** (so `pg_constraint` emits a `contype='f'`
  row), at least one non-PK index, and columns of varied types.

### Outputs

- No product output. The harness produces a pass/fail verdict from the
  exported snapshot ZIP.

## Behaviors

- **Given** a target seeded with an unindexed FK, **when** the full
  collection runs and a snapshot ZIP is exported, **then** the
  `pg_constraints_v1` payload in `query_results.ndjson` contains a row
  with `contype == "f"` (the string, not a number) for that FK.
- **Given** any catalog/schema collector that produced rows, **when**
  the ZIP is read back, **then** each object in that collector's
  `query_results` payload carries the columns its spec declares.
- **Given** any exported `query_runs.ndjson` row with
  `status="success"` and `row_count=N`, **when** joined to
  `query_results.ndjson` by `run_id`, **then** exactly one payload
  exists whose `payload` array holds exactly `N` objects.
- **Given** an internal-`"char"` column (`contype`, `relkind`,
  `relpersistence`, `provolatile`, `prokind`) present in a payload,
  **when** its JSON value is inspected, **then** it is a
  single-character string, never a JSON number.

## Rules

- **OC-R001 — Collect against live PG.** The harness shall run the full
  collection cycle against a real PostgreSQL target, not a mock, and
  export a snapshot ZIP via the production export path
  (`export.Builder.WriteTo`) — never a hand-built payload.
- **OC-R002 — Declared columns present.** For each catalog/schema
  collector whose exported payload is non-empty, every column its spec
  declares shall appear in each payload object. (INV-OUTPUT-CONTRACT.)
- **OC-R003 — Char-type is text.** Every internal-`"char"` column
  (`contype`, `relkind`, `relpersistence`, `provolatile`, `prokind`)
  present in a payload shall decode as a single-character string, never
  a JSON number. The FK seeded by the harness shall produce a
  `pg_constraints_v1` row with `contype == "f"`. (INV-CHAR-TEXT-VERIFIED.)
- **OC-R004 — Status↔payload joinable.** For every exported
  `query_runs` row with `status="success"` / `row_count=N`, exactly one
  `query_results` payload shall be joinable by `run_id` and contain
  exactly `N` objects. (INV-STATUS-PAYLOAD-VERIFIED.)

## Invariants

- **INV-01** — The harness reuses the production collect + export code
  paths (`collector.Collector` + `export.Builder`); it never
  reconstructs snapshot bytes independently, so what it asserts is what
  a real consumer receives.
- **INV-02** — The harness is read-only against the target beyond the
  one-time seed of representative schema, honoring the Signals safety
  model (no writes during collection, no superuser required).
- **INV-03** — The char-type assertion (`contype == "f"`) is a
  regression lock: it fails on the pre-#313 uncast SQL and passes on
  the `::text`-cast SQL.

## Failure conditions

- **FC-01** — A declared column absent from a non-empty payload fails
  OC-R002.
- **FC-02** — An internal-`"char"` column decoding as a JSON number
  (e.g. `contype == 102`) fails OC-R003.
- **FC-03** — A `status="success"`/`row_count=N` run with no joinable
  payload, or a payload whose row count ≠ N, fails OC-R004.

## Constraints

- The test is gated by the `integration` build tag and the
  `SIGNALS_TEST_PG_DSN` env var; it SKIPS locally when unset and RUNS
  in the CI PG-version matrix. It performs no network calls beyond the
  target PostgreSQL connection.

## Non-Functional Requirements

- **NFR-01 — Version coverage.** The harness runs across PG
  14/15/16/17/18 in CI so version-gated column/type differences surface.
- **NFR-02 — Fast-follow scope.** This spec covers catalog/schema
  collectors. Sweeping the remaining (~80) non-schema collectors under
  the same output-contract harness is an explicit follow-up, to be
  filed as a child issue of #314.
