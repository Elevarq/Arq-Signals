# Extension-Absent Emission — Cross-Cutting Specification

Feature: Uniform Reporting of Absent Extensions
Version: 1.0
Type: behavioral
Status: DRAFT

---

## Purpose

Several collectors depend on optional extensions (`pg_stat_statements`,
`vector`, `pg_buffercache`, etc.). When the extension is not
installed, the current behavior is to silently skip the query. This
leaves the analyzer unable to distinguish "the extension is absent"
from "the collector failed" from "the collector was filtered out by
cadence."

This specification defines a uniform positive emission for missing
extensions so the analyzer's evidence-completeness model can reason
about why a signal is unavailable.

## Rule

- When a collector declares `RequiresExtension: X` and `X` is not
  installed on the target, the collector MUST emit a single
  sentinel row instead of being silently dropped.
- The sentinel row has the shape:

```json
{
  "available": false,
  "reason": "extension_not_installed",
  "extension": "X",
  "collector": "pg_X_v1"
}
```

- When the extension IS installed, collectors emit their normal rows;
  no sentinel is included.

## Rationale

- `EvidenceCompleteness` in the analyzer treats an absent key and a
  present-but-empty result identically today; this change lets it
  distinguish "no data because ext missing" from "no data because
  empty result set."
- Detectors can produce precise CoverageNotes ("missing
  pg_stat_statements — install the extension to enable
  query-latency-regression") rather than generic "no evidence."

## Requirements

- EA-R001: The pgqueries filter layer MUST, for any query with
  `RequiresExtension` set where the extension is missing, substitute
  a sentinel-emitting SQL that returns one row with the shape above.
- EA-R002: The sentinel row must be serializable to NDJSON with the
  same pipeline as a normal result.
- EA-R003: A collector spec that uses this mechanism MUST reference
  this specification in its Failure Conditions section.
- EA-R004: The analyzer MUST recognize the sentinel shape and surface
  it through `EvidenceCompleteness` as an explicit
  `ExtensionUnavailable` state (distinct from `CollectorFailed` and
  `CollectorEmpty`).

## Invariants

- INV-EA-01: For a given collector and target, either the sentinel is
  present OR normal rows are present — never both, never neither.
- INV-EA-02: The sentinel shape is versioned: if its schema changes,
  the collector key bumps to `_v2`.

## Failure Conditions

- FC-EA-01: Extension-check query itself fails (permission denied,
  catalog corruption) → collector emits `{"available": false,
  "reason": "extension_check_failed", ...}` and the underlying error
  is recorded in the `QueryRun.Error`.
- FC-EA-02: Extension is present but the required role privileges are
  not → collector emits `{"available": false,
  "reason": "insufficient_privilege", ...}`. Applies especially to
  `pg_stat_statements`, which requires `pg_read_all_stats` or
  membership in `pg_monitor` to see rows from other users.

## Non-Functional Requirements

- NFR-EA-01: The sentinel-emission path MUST NOT require an extra
  round-trip when the extension is present. Extension presence is
  already detected at target-collection setup (`detectExtensions`).
- NFR-EA-02: Sentinels are small (one row) and must not meaningfully
  inflate snapshot size.

## Acceptance Rules

- A traceability test asserts that every collector declared with
  `RequiresExtension` has a sentinel path in the pgqueries layer.
- An integration test against a target without `pg_stat_statements`
  confirms the sentinel row is emitted and ingested correctly.
- The analyzer's coverage report distinguishes the three states:
  `CollectorOK`, `CollectorEmpty`, `ExtensionUnavailable`.
