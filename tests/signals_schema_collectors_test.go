package tests

import (
	"strings"
	"testing"

	"github.com/elevarq/arq-signals/internal/pgqueries"
)

// ---------------------------------------------------------------------------
// Schema Metadata Collectors — Phase 1: pg_constraints_v1, pg_indexes_v1
// ---------------------------------------------------------------------------

var schemaPhase1 = []struct {
	id       string
	category string
}{
	{"pg_constraints_v1", "schema"},
	{"pg_indexes_v1", "schema"},
}

// --- Registration tests ---

func TestSchemaPhase1AllRegistered(t *testing.T) {
	for _, tc := range schemaPhase1 {
		q := pgqueries.ByID(tc.id)
		if q == nil {
			t.Errorf("collector %q is not registered", tc.id)
			continue
		}
		if q.Category != tc.category {
			t.Errorf("collector %q: category=%q, want %q", tc.id, q.Category, tc.category)
		}
	}
}

// --- Linter tests ---

func TestSchemaPhase1AllPassLinter(t *testing.T) {
	for _, tc := range schemaPhase1 {
		q := pgqueries.ByID(tc.id)
		if q == nil {
			t.Errorf("collector %q not registered", tc.id)
			continue
		}
		if err := pgqueries.LintQuery(q.SQL); err != nil {
			t.Errorf("collector %q failed linter: %v", tc.id, err)
		}
	}
}

// --- Cadence tests ---

func TestSchemaPhase1DefaultCadence(t *testing.T) {
	for _, tc := range schemaPhase1 {
		q := pgqueries.ByID(tc.id)
		if q == nil {
			continue
		}
		if q.Cadence != pgqueries.CadenceDaily {
			t.Errorf("collector %q: cadence=%v, want CadenceDaily (24h)", tc.id, q.Cadence)
		}
	}
}

// --- Schema filter tests ---

func TestSchemaPhase1ExcludesSystemSchemas(t *testing.T) {
	systemSchemas := []string{
		"pg_catalog", "information_schema", "pg_toast", "pg_temp_",
	}

	for _, tc := range schemaPhase1 {
		q := pgqueries.ByID(tc.id)
		if q == nil {
			continue
		}
		sql := strings.ToLower(q.SQL)
		for _, schema := range systemSchemas {
			if !strings.Contains(sql, schema) {
				t.Errorf("collector %q SQL must filter out %q", tc.id, schema)
			}
		}
	}
}

// --- Deterministic ordering tests ---

func TestSchemaPhase1HasOrderBy(t *testing.T) {
	for _, tc := range schemaPhase1 {
		q := pgqueries.ByID(tc.id)
		if q == nil {
			continue
		}
		if !containsCI(q.SQL, "ORDER BY") {
			t.Errorf("collector %q must have ORDER BY for deterministic output", tc.id)
		}
	}
}

// --- Output shape: explicit SELECT (no SELECT *) ---

func TestSchemaPhase1NoSelectStar(t *testing.T) {
	for _, tc := range schemaPhase1 {
		q := pgqueries.ByID(tc.id)
		if q == nil {
			continue
		}
		if strings.Contains(q.SQL, "SELECT *") || strings.Contains(q.SQL, "select *") {
			t.Errorf("collector %q must not use SELECT *", tc.id)
		}
	}
}

// --- No duplicate IDs ---

func TestSchemaPhase1NoDuplicateIDs(t *testing.T) {
	seen := make(map[string]bool)
	for _, q := range pgqueries.All() {
		if seen[q.ID] {
			t.Errorf("duplicate collector ID: %q", q.ID)
		}
		seen[q.ID] = true
	}
}

// --- Catalog count increases ---

func TestSchemaPhase1CatalogCount(t *testing.T) {
	all := pgqueries.All()
	// 29 existing + 2 new = 31 minimum
	if len(all) < 31 {
		t.Errorf("catalog has %d collectors, want at least 31 (29 existing + 2 schema)", len(all))
	}
}

// --- pg_constraints_v1 specific ---

func TestConstraintsCollectorOutputColumns(t *testing.T) {
	q := pgqueries.ByID("pg_constraints_v1")
	if q == nil {
		t.Fatal("pg_constraints_v1 not registered")
	}

	sql := strings.ToLower(q.SQL)

	requiredColumns := []string{
		"schemaname", "relname", "conname", "contype", "condef",
		"column_name", "column_position", "relkind", "n_live_tup",
		"confrelname", "confschemaname",
	}

	for _, col := range requiredColumns {
		if !strings.Contains(sql, col) {
			t.Errorf("pg_constraints_v1 must include column %q in output", col)
		}
	}
}

func TestConstraintsCollectorUsesUnnest(t *testing.T) {
	q := pgqueries.ByID("pg_constraints_v1")
	if q == nil {
		t.Fatal("pg_constraints_v1 not registered")
	}
	if !containsCI(q.SQL, "unnest") {
		t.Error("pg_constraints_v1 must use unnest(conkey) for multi-column support")
	}
	if !containsCI(q.SQL, "ordinality") {
		t.Error("pg_constraints_v1 must use WITH ORDINALITY for column_position")
	}
}

func TestConstraintsCollectorOrderByIncludesPosition(t *testing.T) {
	q := pgqueries.ByID("pg_constraints_v1")
	if q == nil {
		t.Fatal("pg_constraints_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)
	orderIdx := strings.LastIndex(sql, "order by")
	if orderIdx < 0 {
		t.Fatal("missing ORDER BY")
	}
	orderClause := sql[orderIdx:]
	if !strings.Contains(orderClause, "ordinality") && !strings.Contains(orderClause, "column_position") {
		t.Error("ORDER BY must include column_position/ordinality for determinism")
	}
}

func TestConstraintsCollectorResultKind(t *testing.T) {
	q := pgqueries.ByID("pg_constraints_v1")
	if q == nil {
		t.Fatal("pg_constraints_v1 not registered")
	}
	if q.ResultKind != pgqueries.ResultRowset {
		t.Errorf("ResultKind: got %q, want rowset", q.ResultKind)
	}
}

// --- pg_indexes_v1 specific ---

func TestIndexesCollectorOutputColumns(t *testing.T) {
	q := pgqueries.ByID("pg_indexes_v1")
	if q == nil {
		t.Fatal("pg_indexes_v1 not registered")
	}

	sql := strings.ToLower(q.SQL)

	requiredColumns := []string{
		"schemaname", "tablename", "indexname", "indexdef", "tablespace",
	}

	for _, col := range requiredColumns {
		if !strings.Contains(sql, col) {
			t.Errorf("pg_indexes_v1 must include column %q in output", col)
		}
	}
}

func TestIndexesCollectorIncludesIndexdef(t *testing.T) {
	q := pgqueries.ByID("pg_indexes_v1")
	if q == nil {
		t.Fatal("pg_indexes_v1 not registered")
	}
	if !containsCI(q.SQL, "indexdef") {
		t.Error("pg_indexes_v1 must include indexdef for leading-column parsing")
	}
}

func TestIndexesCollectorResultKind(t *testing.T) {
	q := pgqueries.ByID("pg_indexes_v1")
	if q == nil {
		t.Fatal("pg_indexes_v1 not registered")
	}
	if q.ResultKind != pgqueries.ResultRowset {
		t.Errorf("ResultKind: got %q, want rowset", q.ResultKind)
	}
}

func TestIndexesCollectorUsesCoalesce(t *testing.T) {
	// tablespace should use COALESCE to return empty string instead of null
	q := pgqueries.ByID("pg_indexes_v1")
	if q == nil {
		t.Fatal("pg_indexes_v1 not registered")
	}
	if !containsCI(q.SQL, "COALESCE") {
		t.Error("pg_indexes_v1 should COALESCE tablespace to empty string")
	}
}

// ---------------------------------------------------------------------------
// Schema Metadata Collectors — Phase 1 Step 3: pg_stats_v1
// ---------------------------------------------------------------------------

// --- pg_stats_v1 registration ---

func TestStatsCollectorRegistered(t *testing.T) {
	q := pgqueries.ByID("pg_stats_v1")
	if q == nil {
		t.Fatal("pg_stats_v1 is not registered")
	}
	if q.Category != "schema" {
		t.Errorf("category: got %q, want %q", q.Category, "schema")
	}
}

// --- pg_stats_v1 linter ---

func TestStatsCollectorPassesLinter(t *testing.T) {
	q := pgqueries.ByID("pg_stats_v1")
	if q == nil {
		t.Fatal("pg_stats_v1 not registered")
	}
	if err := pgqueries.LintQuery(q.SQL); err != nil {
		t.Errorf("pg_stats_v1 failed linter: %v", err)
	}
}

// --- pg_stats_v1 cadence ---

func TestStatsCollectorCadence(t *testing.T) {
	q := pgqueries.ByID("pg_stats_v1")
	if q == nil {
		t.Fatal("pg_stats_v1 not registered")
	}
	if q.Cadence != pgqueries.CadenceDaily {
		t.Errorf("cadence: got %v, want CadenceDaily (24h)", q.Cadence)
	}
}

// --- pg_stats_v1 schema filter ---

func TestStatsCollectorExcludesSystemSchemas(t *testing.T) {
	q := pgqueries.ByID("pg_stats_v1")
	if q == nil {
		t.Fatal("pg_stats_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)
	for _, schema := range []string{"pg_catalog", "information_schema", "pg_toast", "pg_temp_"} {
		if !strings.Contains(sql, schema) {
			t.Errorf("pg_stats_v1 must filter out %q", schema)
		}
	}
}

// --- pg_stats_v1 deterministic ordering ---

func TestStatsCollectorHasOrderBy(t *testing.T) {
	q := pgqueries.ByID("pg_stats_v1")
	if q == nil {
		t.Fatal("pg_stats_v1 not registered")
	}
	if !containsCI(q.SQL, "ORDER BY") {
		t.Error("pg_stats_v1 must have ORDER BY for deterministic output")
	}
}

// --- pg_stats_v1 explicit column list ---

func TestStatsCollectorNoSelectStar(t *testing.T) {
	q := pgqueries.ByID("pg_stats_v1")
	if q == nil {
		t.Fatal("pg_stats_v1 not registered")
	}
	if strings.Contains(q.SQL, "SELECT *") || strings.Contains(q.SQL, "select *") {
		t.Error("pg_stats_v1 must not use SELECT *")
	}
}

// --- pg_stats_v1 required output columns ---

func TestStatsCollectorOutputColumns(t *testing.T) {
	q := pgqueries.ByID("pg_stats_v1")
	if q == nil {
		t.Fatal("pg_stats_v1 not registered")
	}

	sql := strings.ToLower(q.SQL)

	required := []string{
		"schemaname", "tablename", "attname",
		"n_distinct", "correlation", "null_frac", "avg_width",
	}

	for _, col := range required {
		if !strings.Contains(sql, col) {
			t.Errorf("pg_stats_v1 must include column %q", col)
		}
	}
}

// --- pg_stats_v1 excluded columns (data samples) ---

func TestStatsCollectorExcludesDataSamples(t *testing.T) {
	q := pgqueries.ByID("pg_stats_v1")
	if q == nil {
		t.Fatal("pg_stats_v1 not registered")
	}

	sql := strings.ToLower(q.SQL)

	excluded := []string{
		"most_common_vals", "most_common_freqs",
		"histogram_bounds", "most_common_elems",
		"most_common_elem_freqs", "elem_count_histogram",
	}

	for _, col := range excluded {
		if strings.Contains(sql, col) {
			t.Errorf("pg_stats_v1 must NOT include %q (contains data samples)", col)
		}
	}
}

// --- pg_stats_v1 result kind ---

func TestStatsCollectorResultKind(t *testing.T) {
	q := pgqueries.ByID("pg_stats_v1")
	if q == nil {
		t.Fatal("pg_stats_v1 not registered")
	}
	if q.ResultKind != pgqueries.ResultRowset {
		t.Errorf("ResultKind: got %q, want rowset", q.ResultKind)
	}
}

// --- pg_stats_v1 included on PG 14 ---

func TestStatsCollectorIncludedOnPG14(t *testing.T) {
	filtered := pgqueries.Filter(pgqueries.FilterParams{
		PGMajorVersion: 14,
		Extensions:     []string{},
	})
	found := false
	for _, q := range filtered {
		if q.ID == "pg_stats_v1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("pg_stats_v1 must be included on PG 14")
	}
}

// --- Updated catalog count ---

func TestSchemaCatalogCountWithStats(t *testing.T) {
	all := pgqueries.All()
	// 29 existing + 3 schema = 32 minimum
	if len(all) < 32 {
		t.Errorf("catalog has %d collectors, want at least 32", len(all))
	}
}

// --- Version filtering ---

func TestSchemaPhase1IncludedOnPG14(t *testing.T) {
	filtered := pgqueries.Filter(pgqueries.FilterParams{
		PGMajorVersion: 14,
		Extensions:     []string{},
	})
	idSet := make(map[string]bool)
	for _, q := range filtered {
		idSet[q.ID] = true
	}
	for _, tc := range schemaPhase1 {
		if !idSet[tc.id] {
			t.Errorf("collector %q must be included on PG 14", tc.id)
		}
	}
}
