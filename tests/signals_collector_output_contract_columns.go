//go:build integration

// Declared-column derivation for the collector output-contract harness
// (#316). The authoritative source of a collector's declared output
// columns is its specification under specifications/collectors/. This
// file parses those specs at test time so the harness never hand-copies
// a column list that would silently drift from the spec — mirroring how
// #314/#326 derived the char-column assertions from the spec.
//
// A collector's declared columns are the first cell of every row of the
// markdown table(s) that follow a heading beginning "## Output columns"
// in its spec. Most collectors have their own <id>.md; a handful of
// family variants (definition-mode collectors, the MCV stats sibling)
// are documented inside a parent spec — familySpecSources maps those to
// the parent file and merges the base + definition-mode tables.
//
// Collectors whose output is intentionally dynamic (SELECT *, so the
// column set is version-dependent and not enumerable from the spec) are
// listed in dynamicColumnCollectors and are asserted for row presence
// only, never for a fixed declared-column set.
//
// Spec: specifications/collector-output-contract.md (OC-R002).

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dynamicColumnCollectors emit a version-dependent column set via
// SELECT * (documented in their specs as R037 dynamic-column precedent),
// so a fixed declared-column list cannot be asserted. Their row-presence
// and char/status invariants are still covered by the ALL-payload
// sweeps (OC-R003/R004/R005). Each entry carries the reason it is
// column-dynamic.
var dynamicColumnCollectors = map[string]string{
	"pg_version_v1":                        "scalar SELECT version(); single value keyed by the version() output column, no fixed column table",
	"bgwriter_stats_v1":                    "spec-documented dynamic: whatever pg_stat_bgwriter exposes on the target (PG 17 split checkpoints into pg_stat_checkpointer, so the pre-17 columns are absent)",
	"pg_stats_array_range_v1":              "SELECT * over per-column histogram/array-range stats; columns vary by type (opt-in, HighSensitivity)",
	"timescaledb_hypertables_v1":           "SELECT * over timescaledb_information.hypertables; columns drift across TimescaleDB versions (>=2.20 primary_dimension*)",
	"timescaledb_continuous_aggregates_v1": "SELECT * over continuous_aggregates; finalized column <=2.24 only",
	"timescaledb_jobs_v1":                  "SELECT * over jobs; column set drifts across TimescaleDB versions",
	"timescaledb_job_stats_v1":             "SELECT * over job_stats; column set drifts across TimescaleDB versions",
	"timescaledb_job_errors_v1":            "SELECT * over job_errors; column set drifts across TimescaleDB versions",
	"timescaledb_chunks_v1":                "SELECT * over chunks; column set drifts across TimescaleDB versions",
	"timescaledb_chunk_summary_v1":         "SELECT * over chunk summary; column set drifts across TimescaleDB versions",
	"timescaledb_dimensions_v1":            "SELECT * over dimensions; column set drifts across TimescaleDB versions",
	"timescaledb_compression_settings_v1":  "SELECT * over compression_settings; column set drifts across TimescaleDB versions",
	"timescaledb_compression_stats_v1":     "SELECT * over compression stats; column set drifts across TimescaleDB versions",
	"timescaledb_hypertable_sizes_v1":      "SELECT * over hypertable sizes; column set drifts across TimescaleDB versions",
}

// zeroRowAllowlist enumerates the collectors that legitimately emit no
// rows in the seeded live-PG test environment, each with the reason it
// cannot be exercised here. A collector emitting no rows that is NOT on
// this list fails the harness (OC-R002) — so the allowlist is the
// explicit, auditable record of what is NOT covered, never a silent gap.
// Adding a collector to the registry forces a decision: seed a fixture
// that makes it emit rows, or classify it here with a reason.
//
// The reasons fall into four families:
//   - contention: needs a concurrent, contended, or long-lived session
//     the single-connection harness cannot manufacture;
//   - in-flight op: needs an operation running AT sample time
//     (pg_stat_progress_*);
//   - replication: needs a standby / replication slot;
//   - capability-gated: needs an extension or a privilege the
//     least-privilege pg_monitor collector role lacks;
//   - rare object: a catalog object type not worth a bespoke fixture
//     (built-in-only in a fresh cluster).
var zeroRowAllowlist = map[string]string{
	// --- contention / live-session state (no concurrent load in the harness) ---
	"blocking_locks_v1":         "needs a blocked+blocking session pair; the single-connection harness cannot create lock contention",
	"pg_locks_summary_v1":       "needs held locks from concurrent sessions; none exist in the quiescent test target",
	"idle_in_txn_offenders_v1":  "needs a session idle-in-transaction past the threshold; no such session in the harness",
	"long_running_txns_v1":      "needs a transaction open longer than the threshold; the harness runs no long txn",
	"temp_io_pressure_v1":       "needs a backend spilling temp files at sample time; no temp-heavy query runs",
	"pg_stat_user_functions_v1": "needs track_functions enabled AND a tracked function actually called; the harness makes no such call",

	// --- in-flight maintenance operations (pg_stat_progress_*) ---
	"pg_stat_progress_analyze_v1":      "one row only while an ANALYZE is in progress at sample time; none running",
	"pg_stat_progress_vacuum_v1":       "one row only while a VACUUM is in progress at sample time; none running",
	"pg_stat_progress_cluster_v1":      "one row only while a CLUSTER/VACUUM FULL is in progress; none running",
	"pg_stat_progress_create_index_v1": "one row only while a CREATE INDEX/REINDEX is in progress; none running",
	"pg_stat_progress_copy_v1":         "one row only while a COPY is in progress; none running",
	"pg_stat_progress_basebackup_v1":   "one row only while a base backup is streaming; none running",

	// --- replication (no standby / slots in a standalone test server) ---
	"replication_status_v1":        "needs a connected streaming standby (pg_stat_replication); the test server has none",
	"replication_slots_risk_v1":    "needs a replication slot; none created on the test server",
	"pg_stat_replication_slots_v1": "needs a logical replication slot with stats; none created on the test server",

	// --- capability-gated: extension not installed ---
	"pg_stat_statements_v1":                "requires the pg_stat_statements extension (not installed in the CI/local matrix)",
	"pgss_capacity_v1":                     "requires the pg_stat_statements extension (not installed)",
	"pgss_reset_check_v1":                  "requires the pg_stat_statements extension (not installed)",
	"pg_vector_columns_v1":                 "requires the pgvector (vector) extension (not installed)",
	"pg_stats_array_range_v1":              "opt-in HighSensitivity histogram collector (RequiresArrayRangeOptIn); disabled in the harness",
	"timescaledb_extension_v1":             "requires the timescaledb extension (not installed in the matrix)",
	"timescaledb_hypertables_v1":           "requires the timescaledb extension (not installed)",
	"timescaledb_hypertable_sizes_v1":      "requires the timescaledb extension (not installed)",
	"timescaledb_dimensions_v1":            "requires the timescaledb extension (not installed)",
	"timescaledb_chunks_v1":                "requires the timescaledb extension (not installed)",
	"timescaledb_chunk_summary_v1":         "requires the timescaledb extension (not installed)",
	"timescaledb_compression_settings_v1":  "requires the timescaledb extension (not installed)",
	"timescaledb_compression_stats_v1":     "requires the timescaledb extension (not installed)",
	"timescaledb_continuous_aggregates_v1": "requires the timescaledb extension (not installed)",
	"timescaledb_jobs_v1":                  "requires the timescaledb extension (not installed)",
	"timescaledb_job_stats_v1":             "requires the timescaledb extension (not installed)",
	"timescaledb_job_errors_v1":            "requires the timescaledb extension (not installed)",

	// --- capability-gated: privilege the pg_monitor role lacks ---
	"pg_statistic_ext_data_v1":     "reads pg_statistic_ext_data, which has PUBLIC SELECT revoked; a pg_monitor role is skipped (#200)",
	"pg_statistic_ext_data_mcv_v1": "HighSensitivity MCV sibling of pg_statistic_ext_data_v1; also needs privileged read of pg_statistic_ext_data (skipped under pg_monitor)",

	// --- config-gated: needs a non-default server GUC ---
	"pg_prepared_xacts_v1":   "needs a prepared transaction; max_prepared_transactions defaults to 0 so 2PC is unavailable in the matrix",
	"pg_db_role_settings_v1": "needs an ALTER ROLE/DATABASE ... SET; the least-privilege collector role cannot ALTER, and the harness avoids mutating global (non-schema) role/database state",

	// --- rare catalog objects: built-in-only in a fresh cluster, not worth a bespoke fixture ---
	"pg_operators_v1":      "emits only non-extension user-defined operators; a fresh cluster has none and none is seeded (built-ins live in pg_catalog, excluded)",
	"pg_aggregates_v1":     "emits only user-defined aggregates; none seeded (built-ins excluded by schema/OID filter)",
	"pg_casts_v1":          "emits only user-defined casts; none seeded (built-ins excluded by OID < 16384)",
	"pg_collations_v1":     "emits only user-defined collations; none seeded (built-ins excluded)",
	"pg_text_search_v1":    "emits only user-defined text-search configs/dictionaries; none seeded (built-ins excluded)",
	"fdw_user_mappings_v1": "needs a USER MAPPING on the foreign server; the FDW fixture creates a server + foreign table but no user mapping",

	// --- wraparound: no relation is near the freeze thresholds in a fresh cluster ---
	"wraparound_blockers_v1": "flags relations past the autovacuum-freeze age thresholds; a freshly-seeded cluster has none",
}

// specSource identifies one spec file (and optionally which
// "## Output columns ..." table modes within it) whose column table(s)
// contribute to a collector's declared column set. The default for a
// collector is its own <id>.md with every "## Output columns*" table
// merged; family variants documented inside a parent spec are mapped
// explicitly via familySpecSources.
type specSource struct {
	file string // spec filename under specifications/collectors/
	// modes limits which "## Output columns ..." tables are merged. Empty
	// means "all tables under any Output columns heading in the file".
	modes []string
}

// familySpecSources maps the family/variant collectors that lack their
// own <id>.md to the parent spec + the table modes that define their
// columns. Definition-mode variants inherit the parent's inventory
// columns plus the definition-mode additions.
var familySpecSources = map[string][]specSource{
	// Definition-mode variants emit the parent's inventory columns PLUS
	// the "definition mode, adds" columns — modes ["*"] merges both.
	"pg_functions_definitions_v1": {
		{file: "pg_functions_v1.md", modes: []string{"*"}},
	},
	"pg_triggers_definitions_v1": {
		{file: "pg_triggers_v1.md", modes: []string{"*"}},
	},
	"pg_views_definitions_v1": {
		{file: "pg_views_v1.md", modes: []string{"*"}},
	},
	"pg_matviews_definitions_v1": {
		{file: "pg_matviews_v1.md", modes: []string{"*"}},
	},
	"pg_statistic_ext_data_mcv_v1": {
		{file: "pg_statistic_ext_data_v1.md"}, // identical identity + kind_data/available columns
	},
}

// declaredColumnsForCollector derives the declared output columns for a
// collector ID from its spec, or returns ("", nil, false) when the
// collector is column-dynamic (SELECT *). It fails the test if a
// collector has neither a parseable spec nor a dynamic classification —
// there is no silent gap.
func declaredColumnsForCollector(t *testing.T, id string) (cols []string, dynamic bool) {
	t.Helper()
	if reason, ok := dynamicColumnCollectors[id]; ok {
		t.Logf("OC-R002: collector %s is column-dynamic (%s) — asserting row-presence only", id, reason)
		return nil, true
	}

	sources, ok := familySpecSources[id]
	if !ok {
		sources = []specSource{{file: id + ".md"}}
	}

	seen := map[string]bool{}
	for _, s := range sources {
		path := filepath.Join(repoRoot(t), "specifications", "collectors", s.file)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("OC-R002: collector %s: reading spec %s: %v", id, s.file, err)
		}
		for _, c := range parseOutputColumns(string(data), s.modes) {
			if !seen[c] {
				seen[c] = true
				cols = append(cols, c)
			}
		}
	}
	if len(cols) == 0 {
		t.Fatalf("OC-R002: collector %s: no declared output columns parsed from %v — spec missing an '## Output columns' table? (add it, or classify the collector as column-dynamic)", id, sources)
	}
	return cols, false
}

// parseOutputColumns extracts the first-cell column names from every
// markdown table that follows a heading beginning "## Output columns"
// (case-insensitive), up to the next "## " heading. When modes is
// non-empty only headings whose text contains one of the mode strings
// are considered. Backticks and surrounding whitespace are stripped; the
// table's header row (Column/---) and separator rows are skipped.
func parseOutputColumns(body string, modes []string) []string {
	var out []string
	lines := strings.Split(body, "\n")
	inSection := false
	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")))
			if strings.HasPrefix(heading, "output columns") {
				inSection = matchesMode(heading, modes)
			} else {
				inSection = false
			}
			continue
		}
		if !inSection {
			continue
		}
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := strings.Split(trimmed, "|")
		if len(cells) < 2 {
			continue
		}
		name := strings.TrimSpace(cells[1])
		name = strings.Trim(name, "`")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		lc := strings.ToLower(name)
		// Skip the header row and the |---|---| separator row.
		if lc == "column" || lc == "column name" || strings.HasPrefix(name, "-") || strings.HasPrefix(name, ":-") {
			continue
		}
		out = append(out, name)
	}
	return out
}

// matchesMode decides whether an "## Output columns ..." heading's table
// contributes to the derived column set.
//
//   - modes empty (the default for a plain <id>.md collector): include
//     the inventory table(s) but EXCLUDE the "definition mode, adds"
//     table — those columns belong only to the collector's
//     *_definitions_v1 sibling, not the inventory collector itself.
//   - modes == ["*"]: include every Output-columns table (used for the
//     definition-mode variants, which emit inventory + definition
//     columns).
//   - otherwise: include a heading only if it contains one of the mode
//     substrings.
func matchesMode(heading string, modes []string) bool {
	if len(modes) == 0 {
		return !strings.Contains(heading, "definition mode")
	}
	for _, m := range modes {
		if m == "*" {
			return true
		}
		if strings.Contains(heading, strings.ToLower(m)) {
			return true
		}
	}
	return false
}
