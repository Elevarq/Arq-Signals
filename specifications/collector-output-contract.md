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
`pg_functions_definitions_v1`, `pg_proc_config_v1`) **and the remaining
schema collectors that alias an internal-`"char"` column** but were not
exercised by the #314 fixtures (#326): `pg_partitions_v1`
(`partition_strategy`), `pg_triggers_v1` / `pg_triggers_definitions_v1`
(`tg_enabled`), and `fdw_foreign_tables_v1` (`relkind`). The remaining
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
  row), at least one non-PK index, and columns of varied types; plus a
  **partitioned table**, a user-defined **trigger**, a SQL **function**,
  and an **identity column** so the remaining internal-`"char"` schema
  collectors (`pg_partitions_v1`, `pg_triggers_v1` /
  `pg_triggers_definitions_v1`, `pg_functions_v1` /
  `pg_functions_definitions_v1`, `pg_identity_columns_v1`) each emit a
  row. The seed runs as the non-superuser collector role.
- An optional superuser DSN (`SIGNALS_TEST_PG_SUPERUSER_DSN`) used ONLY
  to provision the FDW capability that a `pg_monitor` role cannot create
  itself (`CREATE EXTENSION postgres_fdw` + a foreign `SERVER` +
  `GRANT USAGE` to the collector role). When absent, the FDW fixture and
  its `fdw_foreign_tables_v1.relkind` assertion are capability-gated and
  skipped (OC-R007); every other assertion still runs.

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
- **Given** a target seeded so that `catalog_bloat_v1`/
  `catalog_index_bloat_v1` emit `relkind`, a function emits
  `provolatile`/`volatility`, and an identity column emits
  `attidentity`, **when** the ZIP is read back, **then** each of those
  values is a string (`relkind`/`volatility` a single character;
  `attidentity` the empty string for the non-set case), never a JSON
  number — because the connection boundary decodes OID 18 as text for
  every collector, not just the columns hand-cast with `::text`.
- **Given** a target seeded with a **partitioned table**, a
  user-defined **trigger**, and a SQL **function**, **when** the ZIP is
  read back, **then** `pg_partitions_v1.partition_strategy` (from
  `pg_partitioned_table.partstrat`), `pg_triggers_v1.tg_enabled` /
  `pg_triggers_definitions_v1.tg_enabled` (from `pg_trigger.tgenabled`),
  and `pg_functions_v1.volatility` / `pg_functions_definitions_v1.volatility`
  (from `pg_proc.provolatile`) each decode as a single-character string,
  never a JSON number — extending the OID-18 boundary lock across the
  remaining schema collectors that alias an internal-`"char"` column.
- **Given** the target additionally exposes the FDW capability (a
  `postgres_fdw` foreign server the collector role may create a foreign
  table against), **when** the ZIP is read back, **then**
  `fdw_foreign_tables_v1.relkind` (from `pg_class.relkind`) decodes as
  the single-character string `"f"`, never a JSON number. When the
  environment cannot provide the FDW capability the FDW assertion is
  capability-gated and skipped with a documented reason — the remaining
  schema-collector char assertions still run.

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
- **OC-R005 — Char-type normalized at the connection boundary.** The
  internal PostgreSQL `"char"` type (OID 18) shall be normalized to a
  text string at the collection boundary for ALL collectors and
  queries, present and future — via a pooled-connection type
  registration, not per-column `::text` casts. No OID-18 value shall
  leave a collector as a JSON number, regardless of whether its SQL
  hand-casts the column. In particular the columns #313's per-column
  sweep missed — `relkind` (`catalog_bloat_v1`, `catalog_index_bloat_v1`,
  `catalog_fdw` `fdw_*_v1`), `provolatile`/`volatility`
  (`catalog_schema` functions), and `attidentity` (`catalog_schema`
  identity columns) — shall decode as strings (`""` for the non-set
  `attidentity`, matching `col::text` semantics under the text
  protocol's inability to carry an embedded NUL). This closes the class
  the leaky per-column `::text` approach could not (#319, follow-up to
  #312/#313; root cause of Elevarq/Analyzer#1871).
- **OC-R006 — Char sweep covers the remaining schema collectors.** The
  internal-`"char"` output-column whitelist the harness sweeps shall
  include the aliases the remaining schema collectors expose — not only
  the raw catalog names. Specifically it shall include
  `partition_strategy` (`pg_partitions_v1`, from `partstrat`),
  `tg_enabled` (`pg_triggers_v1` / `pg_triggers_definitions_v1`, from
  `tgenabled`), `volatility` (`pg_functions_v1` /
  `pg_functions_definitions_v1`, the **output alias** of `provolatile`),
  and `attidentity` (`pg_identity_columns_v1`). Because the whitelist
  keys the **exported column name**, the function alias is
  `volatility`, never `provolatile`. The harness shall seed a
  partitioned table, a user trigger, a SQL function, and an identity
  column so each of these collectors emits at least one row, and shall
  assert each aliased char value decodes as a single-character string
  (or the empty string for the non-set `attidentity`), never a JSON
  number. This regression-locks #319 for the whole char family across
  the schema collectors #314 missed (#326).
- **OC-R007 — FDW char is capability-gated.** When the environment
  provides the FDW capability — a `postgres_fdw` foreign server the
  collector role may create a foreign table against — the harness shall
  seed a foreign table so `fdw_foreign_tables_v1` emits a row and shall
  assert its `relkind` decodes as the single-character string `"f"`.
  When the capability is absent (no superuser provisioning of the
  extension/server) the FDW assertion is skipped with a documented
  reason; the OC-R006 schema-collector assertions still run. (#326.)

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
- **INV-04** — The OC-R005 assertions (`relkind`, `provolatile`/
  `volatility`, `attidentity` as strings) are a regression lock on the
  central boundary fix: they fail with the OID-18 `AfterConnect`
  registration removed (columns #313 left uncast decode as numbers) and
  pass with it in place, independent of the redundant per-column
  `::text` casts.
- **INV-05** — The OC-R006 schema-collector char assertions
  (`partition_strategy`, `tg_enabled`, `volatility`) and, when the
  capability is present, the OC-R007 `fdw_foreign_tables_v1.relkind`
  assertion are the same regression lock extended across the remaining
  schema collectors: each aliased internal-`"char"` value decodes as a
  single-character string, never a JSON number, so reverting the OID-18
  `AfterConnect` registration turns them RED. The FDW leg is
  capability-gated — its absence never fails the run, but when the
  capability is present the assertion is mandatory.
- **INV-06** — The FDW-capability provisioning uses a separate,
  optional superuser DSN and touches only a dedicated FDW server + a
  `GRANT USAGE`; collection itself still runs as the non-superuser
  collector role (INV-02 holds — no writes and no superuser during
  collection).

## Failure conditions

- **FC-01** — A declared column absent from a non-empty payload fails
  OC-R002.
- **FC-02** — An internal-`"char"` column decoding as a JSON number
  (e.g. `contype == 102`) fails OC-R003.
- **FC-03** — A `status="success"`/`row_count=N` run with no joinable
  payload, or a payload whose row count ≠ N, fails OC-R004.
- **FC-04** — An aliased schema-collector internal-`"char"` value
  (`partition_strategy`, `tg_enabled`, `volatility`, or, when the FDW
  capability is present, `fdw_foreign_tables_v1.relkind`) decoding as a
  JSON number, or one of the seeded schema collectors emitting no such
  value at all, fails OC-R006 / OC-R007.

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
