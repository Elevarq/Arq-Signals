# pg_settings_v1 — Collector Specification

## Purpose

Current values of cluster-level GUCs. Every cost-configuration
recommendation references the current setting so advice is phrased
as a delta ("raise from X to Y") rather than an absolute
prescription.

## Catalog source

- `pg_settings`

## Output columns

| Column | Type | Description |
|---|---|---|
| name | text | GUC name |
| setting | text | Current value (canonical text form) |
| unit | text | Unit of measurement (`ms`, `kB`, `8kB`, etc.) |
| category | text | Configuration category |
| source | text | `default`, `configuration file`, `command line`, ... |
| pending_restart | boolean | Change pending a restart |

## Scope filter

All rows emitted. No filter.

## Invariants

- Deterministic ordering: `ORDER BY name`.
- Stable output column order.
- Read-only query, passes linter.
- `setting` is the effective value (not a formula) — analyzer does
  not need to resolve unit conversion at collection time.

## Failure Conditions

- FC-01: On managed platforms (RDS parameter groups, Cloud SQL
  flags, Azure server parameters), some advanced metadata fields
  visible via `pg_settings` locally may be restricted. The current
  column set is cross-platform safe.
- FC-02: Permission denied (rare — `pg_settings` is world-readable)
  → standard collector error path.

## Configuration

- Category: server
- Cadence: 6h (Cadence6h)
- Retention: RetentionLong
- Min PG version: 10
- Requires extension: none
- Semantics: snapshot
- Enabled by default: yes

## Sensitivity

Low. GUC values are visible to any connected role. No credentials
surface here.

## Analyzer requirements unblocked

- `io-cost-calibration` — baseline for the four target GUCs
  (`random_page_cost`, `seq_page_cost`, `effective_io_concurrency`,
  `effective_cache_size`).
- `object-parameter-drift` — cluster defaults for comparison against
  per-table / per-tablespace / per-function overrides.
- Every recommendation detector — grounds advice in current state.

## Known gap vs aspirational spec

Detectors downstream of this collector benefit from additional GUC
metadata (`min_val`, `max_val`, `boot_val`, `reset_val`, `vartype`,
`context`, `short_desc`, `sourcefile`, `sourceline`, `enumvals`).
These are readable from `pg_settings` on all supported PG versions
but are not in the current SELECT list. **Extension plan:** if the
`io-cost-calibration` implementation requires them, extend the
SELECT list in a separate change — no schema bump needed because
new columns are additive for downstream consumers.
