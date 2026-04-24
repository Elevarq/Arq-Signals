# pg_vector_columns_v1 — Collector Specification

## Purpose

Enumerate pgvector columns: type, declared dimension, average stored
width, storage mode, compression setting, and index coverage. Feeds
the `vector-column-storage` detector and the vector-specific overlay
of the `toast-planner-blindspot` detector.

## Catalog source

- `pg_attribute` joined with `pg_class`, `pg_namespace`, `pg_type`
- `pg_stats` (LEFT JOIN) for `avg_width`
- `pg_index` + `pg_am` for index coverage

Filter: `pg_type.typname IN ('vector', 'halfvec', 'sparsevec')`.

## Output columns

| Column | Type | Description |
|---|---|---|
| relid | oid | Table OID |
| schemaname | text | Schema |
| relname | text | Table name |
| attname | text | Column name |
| atttypname | text | `vector`, `halfvec`, or `sparsevec` |
| dimension | int | From `atttypmod`; NULL if unbounded |
| avg_width | int | Average stored width in bytes; NULL if no stats |
| attstorage | text | `p` / `e` / `x` / `m` |
| attcompression | text | `\0` / `p` / `l` (PG14+) |
| likely_toasted | boolean | Derived: `attstorage IN ('e','x') AND COALESCE(avg_width,0) > 2000` |
| has_index | boolean | True if any index references this column |
| index_types | text[] | Access methods covering this column (e.g. `{hnsw,ivfflat,btree}`) |

## Scope filter

- Only columns whose type is in the pgvector family (resolved by
  `pg_type.typname` so the collector survives pgvector extension
  upgrades).
- User schemas only (excludes `pg_catalog`, `information_schema`,
  `pg_toast`, `pg_temp_%`, `pg_toast_temp_%`).
- `attnum > 0` and `NOT attisdropped`.

## Invariants

- Deterministic ordering: `ORDER BY schemaname, relname, attnum`.
- Stable output column order.
- Read-only, passes linter.
- `likely_toasted` is emitted explicitly so the analyzer does not
  duplicate the derivation.

## Failure Conditions

- FC-01: **pgvector extension not installed** → collector is
  filtered out at the pgqueries layer via `RequiresExtension: vector`.
  No query executes, no sentinel is emitted, no error is raised.
  Absence of output is the signal. This matches the normal case:
  pgvector may be installed on some targets and not others.
- FC-02: **pgvector installed but no column uses a vector type** →
  empty result. This is a normal state — extensions can be installed
  without being used — and must not be treated as a failure.
- FC-03: PG < 14 → filtered out via `MinPGVersion: 14`. The
  `attcompression` column was introduced in PG 14; earlier versions
  would require a separate collector variant, which is out of scope
  for now.
- FC-04: Permission denied on `pg_attribute` / `pg_stats` (rare) →
  standard collector error path.

## Configuration

- Category: schema
- Cadence: 24h (CadenceDaily)
- Retention: RetentionMedium
- Min PG version: 14
- Requires extension: vector
- Semantics: snapshot
- Enabled by default: yes

## Sensitivity

Low. Column metadata only.

## Analyzer requirements unblocked

- `vector-column-storage` — primary evidence.
- `toast-planner-blindspot` overlay — identifies specific vector
  columns driving TOAST amplification on affected tables.
- Storage advice — candidates for `SET STORAGE EXTERNAL` (skip
  compression on high-entropy float data).
