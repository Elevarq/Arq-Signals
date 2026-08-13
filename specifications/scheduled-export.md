# Scheduled auto-export (SE)

- **Prefix:** `SE`
- **Issue:** [#350](https://github.com/Elevarq/Signals/issues/350)
- **Status:** ACTIVE

## Purpose

Signals collects and stores its data on a schedule. Scheduled auto-export
adds the ability to **write the latest snapshot export to a configured file
location on that schedule**, so the destination always holds a fresh export
with no per-cycle operator action.

This is a self-contained Signals capability: **collect → store →
export-to-a-file-location, repeated on the collection schedule.** What (if
anything) consumes the exported files is out of scope; Signals has no
knowledge of any downstream consumer.

## Configuration

| Setting | Env | YAML | Default | Meaning |
|---|---|---|---|---|
| Enable | `SIGNALS_EXPORT_ON_COLLECT` | `export_on_collect` | `false` | Write an export after each collection cycle. |
| Destination | `SIGNALS_EXPORT_DEST` | `export_dest` | `""` | Directory to write exports into. |

Scheduled export runs only when enabled **and** a non-empty destination is
set; otherwise it is a no-op (existing behaviour unchanged).

## Rules

- **SE-R010** — When enabled, Signals writes the **latest-snapshot** export
  (the default export scope — the newest snapshot per target, the same scope
  a filterless on-demand export uses) to the destination **after every
  completed collection cycle**: the initial baseline cycle, each scheduled
  poll, and each on-demand collection.
- **SE-R011** — Each export is written to a **flat file** in the destination
  directory named `<instance-id>-<UTC-timestamp>.zip` (no subdirectories).
  The instance-id component disambiguates several Signals instances writing
  to one shared directory; the nanosecond timestamp makes each file unique
  so a new export never overwrites a prior one. The instance-id is sanitized
  to `[A-Za-z0-9._-]` (other runs of characters collapse to `_`); a blank
  instance-id falls back to `signals`.
- **SE-R012** — The write is **atomic**: the export is written to a temporary
  file and renamed into place, so a process watching the destination never
  observes a partially-written ZIP. A temporary file is never left behind on
  success or failure.
- **SE-R013** — A failed export (build error, unwritable destination) is
  **logged and skipped**; it MUST NOT disrupt or fail the collection cycle.

## Invariants

- **SE-INV-01** — Scheduled export never changes what is collected or stored,
  nor the export ZIP's contents; it only writes the already-defined
  latest-snapshot export to a file on the collection schedule.
- **SE-INV-02** — Files are flat in the destination (no per-target
  subdirectories), so a flat-directory consumer sees every export.

## Acceptance cases

- **SE-AC-1 (normal)** — With `SIGNALS_EXPORT_ON_COLLECT=true` and
  `SIGNALS_EXPORT_DEST=<dir>`, a fresh `<instance>-<ts>.zip` appears in
  `<dir>` after each collection cycle, unattended.
- **SE-AC-2 (boundary — shared destination)** — Two Signals instances with
  distinct instance-ids exporting to one directory never collide (distinct
  filename prefixes); distinct cycles never overwrite (distinct timestamps).
- **SE-AC-3 (failure)** — When the export build fails or the destination is
  unwritable, no partial/temp file remains and the collection cycle
  completes normally (SE-R012/SE-R013).
- **SE-AC-4 (disabled)** — With the feature off (default), no files are
  written and behaviour is byte-identical to before.
