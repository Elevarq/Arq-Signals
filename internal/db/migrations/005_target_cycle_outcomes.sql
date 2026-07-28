-- Migration: persisted last-cycle outcome per target (SIGNALS-R126).
--
-- Before this migration, a collection cycle that failed entirely for an
-- enabled target (cycleStatus == "failed": target unreachable, auth
-- failure, role-safety failure, or any hard error that aborts the cycle
-- before a snapshot is persisted) was LOG-ONLY. Nothing durable was
-- written, so the export could not distinguish "the collection BROKE"
-- from "nothing collected yet" — it served a clean, empty ZIP either
-- way (the FC-05 false-clean gap, #340).
--
-- This table records the outcome of the most recent cycle per target so
-- a later export (SIGNALS-R125) can refuse a no-successful-data default
-- export with an actionable reason. It is keyed by target_name (the
-- stable identity the collector already logs) and upserted every cycle:
-- a failed cycle records status='failed' + a bounded category; a
-- successful cycle records status='success' and clears the category,
-- so the marker always reflects the LAST outcome.
--
-- The record is metadata only — target name, status, bounded category,
-- timestamp. It carries no credential, DSN, host, or secret material
-- (INV-SIGNALS-07). The `category` is drawn from the fixed-cardinality
-- failure enum already used for metrics (connect_error, safety_check,
-- version_unsupported, timeout_setup, persistence, internal) so it
-- leaks no raw error text.

CREATE TABLE IF NOT EXISTS target_cycle_outcomes (
    target_name TEXT PRIMARY KEY,
    status      TEXT NOT NULL,
    category    TEXT NOT NULL DEFAULT '',
    updated_at  TEXT NOT NULL
);
