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

// ---------------------------------------------------------------------------
// Schema Metadata Collectors — Phase 1 Step 4: pg_columns_v1
// ---------------------------------------------------------------------------

// --- pg_columns_v1 registration ---

func TestColumnsCollectorRegistered(t *testing.T) {
	q := pgqueries.ByID("pg_columns_v1")
	if q == nil {
		t.Fatal("pg_columns_v1 is not registered")
	}
	if q.Category != "schema" {
		t.Errorf("category: got %q, want %q", q.Category, "schema")
	}
}

func TestColumnsCollectorPassesLinter(t *testing.T) {
	q := pgqueries.ByID("pg_columns_v1")
	if q == nil {
		t.Fatal("pg_columns_v1 not registered")
	}
	if err := pgqueries.LintQuery(q.SQL); err != nil {
		t.Errorf("pg_columns_v1 failed linter: %v", err)
	}
}

func TestColumnsCollectorCadence(t *testing.T) {
	q := pgqueries.ByID("pg_columns_v1")
	if q == nil {
		t.Fatal("pg_columns_v1 not registered")
	}
	if q.Cadence != pgqueries.CadenceDaily {
		t.Errorf("cadence: got %v, want CadenceDaily (24h)", q.Cadence)
	}
}

func TestColumnsCollectorExcludesSystemSchemas(t *testing.T) {
	q := pgqueries.ByID("pg_columns_v1")
	if q == nil {
		t.Fatal("pg_columns_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)
	for _, schema := range []string{"pg_catalog", "information_schema", "pg_toast", "pg_temp_"} {
		if !strings.Contains(sql, schema) {
			t.Errorf("pg_columns_v1 must filter out %q", schema)
		}
	}
}

func TestColumnsCollectorHasOrderBy(t *testing.T) {
	q := pgqueries.ByID("pg_columns_v1")
	if q == nil {
		t.Fatal("pg_columns_v1 not registered")
	}
	if !containsCI(q.SQL, "ORDER BY") {
		t.Error("pg_columns_v1 must have ORDER BY for deterministic output")
	}
}

func TestColumnsCollectorNoSelectStar(t *testing.T) {
	q := pgqueries.ByID("pg_columns_v1")
	if q == nil {
		t.Fatal("pg_columns_v1 not registered")
	}
	if strings.Contains(q.SQL, "SELECT *") || strings.Contains(q.SQL, "select *") {
		t.Error("pg_columns_v1 must not use SELECT *")
	}
}

func TestColumnsCollectorOutputColumns(t *testing.T) {
	q := pgqueries.ByID("pg_columns_v1")
	if q == nil {
		t.Fatal("pg_columns_v1 not registered")
	}

	sql := strings.ToLower(q.SQL)

	required := []string{
		"schemaname", "relname", "attname", "attnum",
		"typname", "is_nullable", "has_default", "attlen",
	}

	for _, col := range required {
		if !strings.Contains(sql, col) {
			t.Errorf("pg_columns_v1 must include column %q", col)
		}
	}
}

func TestColumnsCollectorUsesFormatType(t *testing.T) {
	q := pgqueries.ByID("pg_columns_v1")
	if q == nil {
		t.Fatal("pg_columns_v1 not registered")
	}
	if !containsCI(q.SQL, "format_type") {
		t.Error("pg_columns_v1 must use format_type() for human-readable type names")
	}
}

func TestColumnsCollectorUsesPgAttribute(t *testing.T) {
	q := pgqueries.ByID("pg_columns_v1")
	if q == nil {
		t.Fatal("pg_columns_v1 not registered")
	}
	if !containsCI(q.SQL, "pg_attribute") {
		t.Error("pg_columns_v1 must use pg_attribute (PostgreSQL-native catalog)")
	}
}

func TestColumnsCollectorExcludesDefaultText(t *testing.T) {
	q := pgqueries.ByID("pg_columns_v1")
	if q == nil {
		t.Fatal("pg_columns_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)
	// pg_get_expr would extract the default expression text — must not appear
	if strings.Contains(sql, "pg_get_expr") {
		t.Error("pg_columns_v1 must NOT use pg_get_expr (default expression may contain sensitive values)")
	}
	// column_default from information_schema is also not allowed
	if strings.Contains(sql, "column_default") {
		t.Error("pg_columns_v1 must NOT include column_default text")
	}
}

func TestColumnsCollectorExcludesSystemColumns(t *testing.T) {
	q := pgqueries.ByID("pg_columns_v1")
	if q == nil {
		t.Fatal("pg_columns_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)
	// Must filter attnum > 0 (system columns like ctid, xmin have attnum <= 0)
	if !strings.Contains(sql, "attnum > 0") {
		t.Error("pg_columns_v1 must filter attnum > 0 to exclude system columns")
	}
}

func TestColumnsCollectorExcludesDroppedColumns(t *testing.T) {
	q := pgqueries.ByID("pg_columns_v1")
	if q == nil {
		t.Fatal("pg_columns_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)
	if !strings.Contains(sql, "attisdropped") {
		t.Error("pg_columns_v1 must filter out dropped columns")
	}
}

func TestColumnsCollectorResultKind(t *testing.T) {
	q := pgqueries.ByID("pg_columns_v1")
	if q == nil {
		t.Fatal("pg_columns_v1 not registered")
	}
	if q.ResultKind != pgqueries.ResultRowset {
		t.Errorf("ResultKind: got %q, want rowset", q.ResultKind)
	}
}

func TestColumnsCollectorIncludedOnPG14(t *testing.T) {
	filtered := pgqueries.Filter(pgqueries.FilterParams{
		PGMajorVersion: 14,
		Extensions:     []string{},
	})
	found := false
	for _, q := range filtered {
		if q.ID == "pg_columns_v1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("pg_columns_v1 must be included on PG 14")
	}
}

// ---------------------------------------------------------------------------
// Schema Metadata Collectors — Phase 2 Step 5: pg_schemas_v1
// ---------------------------------------------------------------------------

func TestSchemasCollectorRegistered(t *testing.T) {
	q := pgqueries.ByID("pg_schemas_v1")
	if q == nil {
		t.Fatal("pg_schemas_v1 is not registered")
	}
	if q.Category != "schema" {
		t.Errorf("category: got %q, want %q", q.Category, "schema")
	}
}

func TestSchemasCollectorPassesLinter(t *testing.T) {
	q := pgqueries.ByID("pg_schemas_v1")
	if q == nil {
		t.Fatal("pg_schemas_v1 not registered")
	}
	if err := pgqueries.LintQuery(q.SQL); err != nil {
		t.Errorf("pg_schemas_v1 failed linter: %v", err)
	}
}

func TestSchemasCollectorCadence(t *testing.T) {
	q := pgqueries.ByID("pg_schemas_v1")
	if q == nil {
		t.Fatal("pg_schemas_v1 not registered")
	}
	if q.Cadence != pgqueries.CadenceDaily {
		t.Errorf("cadence: got %v, want CadenceDaily (24h)", q.Cadence)
	}
}

func TestSchemasCollectorExcludesSystemSchemas(t *testing.T) {
	q := pgqueries.ByID("pg_schemas_v1")
	if q == nil {
		t.Fatal("pg_schemas_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)
	for _, schema := range []string{"pg_catalog", "information_schema", "pg_toast", "pg_temp_"} {
		if !strings.Contains(sql, schema) {
			t.Errorf("pg_schemas_v1 must filter out %q", schema)
		}
	}
}

func TestSchemasCollectorHasOrderBy(t *testing.T) {
	q := pgqueries.ByID("pg_schemas_v1")
	if q == nil {
		t.Fatal("pg_schemas_v1 not registered")
	}
	if !containsCI(q.SQL, "ORDER BY") {
		t.Error("pg_schemas_v1 must have ORDER BY for deterministic output")
	}
}

func TestSchemasCollectorNoSelectStar(t *testing.T) {
	q := pgqueries.ByID("pg_schemas_v1")
	if q == nil {
		t.Fatal("pg_schemas_v1 not registered")
	}
	if strings.Contains(q.SQL, "SELECT *") || strings.Contains(q.SQL, "select *") {
		t.Error("pg_schemas_v1 must not use SELECT *")
	}
}

func TestSchemasCollectorOutputColumns(t *testing.T) {
	q := pgqueries.ByID("pg_schemas_v1")
	if q == nil {
		t.Fatal("pg_schemas_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)

	for _, col := range []string{"nspname", "nspowner", "is_default"} {
		if !strings.Contains(sql, col) {
			t.Errorf("pg_schemas_v1 must include column %q", col)
		}
	}
}

func TestSchemasCollectorUsesPgNamespace(t *testing.T) {
	q := pgqueries.ByID("pg_schemas_v1")
	if q == nil {
		t.Fatal("pg_schemas_v1 not registered")
	}
	if !containsCI(q.SQL, "pg_namespace") {
		t.Error("pg_schemas_v1 must use pg_namespace")
	}
}

func TestSchemasCollectorJoinsRoles(t *testing.T) {
	q := pgqueries.ByID("pg_schemas_v1")
	if q == nil {
		t.Fatal("pg_schemas_v1 not registered")
	}
	if !containsCI(q.SQL, "pg_roles") {
		t.Error("pg_schemas_v1 must join pg_roles for owner name")
	}
}

func TestSchemasCollectorResultKind(t *testing.T) {
	q := pgqueries.ByID("pg_schemas_v1")
	if q == nil {
		t.Fatal("pg_schemas_v1 not registered")
	}
	if q.ResultKind != pgqueries.ResultRowset {
		t.Errorf("ResultKind: got %q, want rowset", q.ResultKind)
	}
}

func TestSchemasCollectorIncludedOnPG14(t *testing.T) {
	filtered := pgqueries.Filter(pgqueries.FilterParams{
		PGMajorVersion: 14,
		Extensions:     []string{},
	})
	found := false
	for _, q := range filtered {
		if q.ID == "pg_schemas_v1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("pg_schemas_v1 must be included on PG 14")
	}
}

// ---------------------------------------------------------------------------
// Schema Metadata Collectors — Phase 2 Step 6: pg_views_v1
// ---------------------------------------------------------------------------

func TestViewsCollectorRegistered(t *testing.T) {
	q := pgqueries.ByID("pg_views_v1")
	if q == nil {
		t.Fatal("pg_views_v1 is not registered")
	}
	if q.Category != "schema" {
		t.Errorf("category: got %q, want %q", q.Category, "schema")
	}
}

func TestViewsCollectorPassesLinter(t *testing.T) {
	q := pgqueries.ByID("pg_views_v1")
	if q == nil {
		t.Fatal("pg_views_v1 not registered")
	}
	if err := pgqueries.LintQuery(q.SQL); err != nil {
		t.Errorf("pg_views_v1 failed linter: %v", err)
	}
}

func TestViewsCollectorCadence(t *testing.T) {
	q := pgqueries.ByID("pg_views_v1")
	if q == nil {
		t.Fatal("pg_views_v1 not registered")
	}
	if q.Cadence != pgqueries.CadenceDaily {
		t.Errorf("cadence: got %v, want CadenceDaily (24h)", q.Cadence)
	}
}

func TestViewsCollectorExcludesSystemSchemas(t *testing.T) {
	q := pgqueries.ByID("pg_views_v1")
	if q == nil {
		t.Fatal("pg_views_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)
	for _, schema := range []string{"pg_catalog", "information_schema", "pg_toast", "pg_temp_"} {
		if !strings.Contains(sql, schema) {
			t.Errorf("pg_views_v1 must filter out %q", schema)
		}
	}
}

func TestViewsCollectorHasOrderBy(t *testing.T) {
	q := pgqueries.ByID("pg_views_v1")
	if q == nil {
		t.Fatal("pg_views_v1 not registered")
	}
	if !containsCI(q.SQL, "ORDER BY") {
		t.Error("pg_views_v1 must have ORDER BY for deterministic output")
	}
}

func TestViewsCollectorNoSelectStar(t *testing.T) {
	q := pgqueries.ByID("pg_views_v1")
	if q == nil {
		t.Fatal("pg_views_v1 not registered")
	}
	if strings.Contains(q.SQL, "SELECT *") || strings.Contains(q.SQL, "select *") {
		t.Error("pg_views_v1 must not use SELECT *")
	}
}

func TestViewsCollectorInventoryColumns(t *testing.T) {
	q := pgqueries.ByID("pg_views_v1")
	if q == nil {
		t.Fatal("pg_views_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)

	for _, col := range []string{"schemaname", "viewname", "viewowner"} {
		if !strings.Contains(sql, col) {
			t.Errorf("pg_views_v1 must include column %q", col)
		}
	}
}

func TestViewsCollectorInventoryModeNoDefinition(t *testing.T) {
	// Default (inventory) mode must NOT include view definition text
	q := pgqueries.ByID("pg_views_v1")
	if q == nil {
		t.Fatal("pg_views_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)

	if strings.Contains(sql, "pg_get_viewdef") {
		t.Error("inventory mode must not include pg_get_viewdef (definition text)")
	}
	if strings.Contains(sql, "definition") {
		// Check it's not an alias — "definition" as an output column
		// Would appear as "AS definition" in the SQL
		if strings.Contains(sql, "as definition") {
			t.Error("inventory mode must not include a 'definition' output column")
		}
	}
}

func TestViewsCollectorUsesPgViews(t *testing.T) {
	q := pgqueries.ByID("pg_views_v1")
	if q == nil {
		t.Fatal("pg_views_v1 not registered")
	}
	if !containsCI(q.SQL, "pg_views") {
		t.Error("pg_views_v1 must query from pg_views")
	}
}

func TestViewsCollectorResultKind(t *testing.T) {
	q := pgqueries.ByID("pg_views_v1")
	if q == nil {
		t.Fatal("pg_views_v1 not registered")
	}
	if q.ResultKind != pgqueries.ResultRowset {
		t.Errorf("ResultKind: got %q, want rowset", q.ResultKind)
	}
}

func TestViewsCollectorIncludedOnPG14(t *testing.T) {
	filtered := pgqueries.Filter(pgqueries.FilterParams{
		PGMajorVersion: 14,
		Extensions:     []string{},
	})
	found := false
	for _, q := range filtered {
		if q.ID == "pg_views_v1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("pg_views_v1 must be included on PG 14")
	}
}

// --- Definition mode collector ---

func TestViewsDefinitionsCollectorRegistered(t *testing.T) {
	q := pgqueries.ByID("pg_views_definitions_v1")
	if q == nil {
		t.Fatal("pg_views_definitions_v1 is not registered")
	}
	if q.Category != "schema" {
		t.Errorf("category: got %q, want %q", q.Category, "schema")
	}
}

func TestViewsDefinitionsCollectorPassesLinter(t *testing.T) {
	q := pgqueries.ByID("pg_views_definitions_v1")
	if q == nil {
		t.Fatal("pg_views_definitions_v1 not registered")
	}
	if err := pgqueries.LintQuery(q.SQL); err != nil {
		t.Errorf("pg_views_definitions_v1 failed linter: %v", err)
	}
}

func TestViewsDefinitionsCollectorIncludesDefinition(t *testing.T) {
	q := pgqueries.ByID("pg_views_definitions_v1")
	if q == nil {
		t.Fatal("pg_views_definitions_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)

	// Must include all inventory columns plus definition
	for _, col := range []string{"schemaname", "viewname", "viewowner", "definition"} {
		if !strings.Contains(sql, col) {
			t.Errorf("pg_views_definitions_v1 must include column %q", col)
		}
	}
}

// ---------------------------------------------------------------------------
// Schema Metadata Collectors — Phase 2 Step 7: pg_matviews_v1
// ---------------------------------------------------------------------------

// --- pg_matviews_v1 inventory mode ---

func TestMatviewsCollectorRegistered(t *testing.T) {
	q := pgqueries.ByID("pg_matviews_v1")
	if q == nil {
		t.Fatal("pg_matviews_v1 is not registered")
	}
	if q.Category != "schema" {
		t.Errorf("category: got %q, want %q", q.Category, "schema")
	}
}

func TestMatviewsCollectorPassesLinter(t *testing.T) {
	q := pgqueries.ByID("pg_matviews_v1")
	if q == nil {
		t.Fatal("pg_matviews_v1 not registered")
	}
	if err := pgqueries.LintQuery(q.SQL); err != nil {
		t.Errorf("pg_matviews_v1 failed linter: %v", err)
	}
}

func TestMatviewsCollectorCadence(t *testing.T) {
	q := pgqueries.ByID("pg_matviews_v1")
	if q == nil {
		t.Fatal("pg_matviews_v1 not registered")
	}
	if q.Cadence != pgqueries.CadenceDaily {
		t.Errorf("cadence: got %v, want CadenceDaily (24h)", q.Cadence)
	}
}

func TestMatviewsCollectorExcludesSystemSchemas(t *testing.T) {
	q := pgqueries.ByID("pg_matviews_v1")
	if q == nil {
		t.Fatal("pg_matviews_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)
	for _, schema := range []string{"pg_catalog", "information_schema", "pg_toast", "pg_temp_"} {
		if !strings.Contains(sql, schema) {
			t.Errorf("pg_matviews_v1 must filter out %q", schema)
		}
	}
}

func TestMatviewsCollectorHasOrderBy(t *testing.T) {
	q := pgqueries.ByID("pg_matviews_v1")
	if q == nil {
		t.Fatal("pg_matviews_v1 not registered")
	}
	if !containsCI(q.SQL, "ORDER BY") {
		t.Error("pg_matviews_v1 must have ORDER BY for deterministic output")
	}
}

func TestMatviewsCollectorNoSelectStar(t *testing.T) {
	q := pgqueries.ByID("pg_matviews_v1")
	if q == nil {
		t.Fatal("pg_matviews_v1 not registered")
	}
	if strings.Contains(q.SQL, "SELECT *") || strings.Contains(q.SQL, "select *") {
		t.Error("pg_matviews_v1 must not use SELECT *")
	}
}

func TestMatviewsCollectorInventoryColumns(t *testing.T) {
	q := pgqueries.ByID("pg_matviews_v1")
	if q == nil {
		t.Fatal("pg_matviews_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)

	for _, col := range []string{"schemaname", "matviewname", "matviewowner", "ispopulated", "hasindexes"} {
		if !strings.Contains(sql, col) {
			t.Errorf("pg_matviews_v1 must include column %q", col)
		}
	}
}

func TestMatviewsCollectorInventoryModeNoDefinition(t *testing.T) {
	q := pgqueries.ByID("pg_matviews_v1")
	if q == nil {
		t.Fatal("pg_matviews_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)

	if strings.Contains(sql, "pg_get_viewdef") {
		t.Error("inventory mode must not include pg_get_viewdef")
	}
	if strings.Contains(sql, "as definition") {
		t.Error("inventory mode must not include a 'definition' output column")
	}
}

func TestMatviewsCollectorUsesPgMatviews(t *testing.T) {
	q := pgqueries.ByID("pg_matviews_v1")
	if q == nil {
		t.Fatal("pg_matviews_v1 not registered")
	}
	if !containsCI(q.SQL, "pg_matviews") {
		t.Error("pg_matviews_v1 must query from pg_matviews")
	}
}

func TestMatviewsCollectorResultKind(t *testing.T) {
	q := pgqueries.ByID("pg_matviews_v1")
	if q == nil {
		t.Fatal("pg_matviews_v1 not registered")
	}
	if q.ResultKind != pgqueries.ResultRowset {
		t.Errorf("ResultKind: got %q, want rowset", q.ResultKind)
	}
}

func TestMatviewsCollectorIncludedOnPG14(t *testing.T) {
	filtered := pgqueries.Filter(pgqueries.FilterParams{
		PGMajorVersion: 14,
		Extensions:     []string{},
	})
	found := false
	for _, q := range filtered {
		if q.ID == "pg_matviews_v1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("pg_matviews_v1 must be included on PG 14")
	}
}

// --- pg_matviews_definitions_v1 definition mode ---

func TestMatviewsDefinitionsCollectorRegistered(t *testing.T) {
	q := pgqueries.ByID("pg_matviews_definitions_v1")
	if q == nil {
		t.Fatal("pg_matviews_definitions_v1 is not registered")
	}
	if q.Category != "schema" {
		t.Errorf("category: got %q, want %q", q.Category, "schema")
	}
}

func TestMatviewsDefinitionsCollectorPassesLinter(t *testing.T) {
	q := pgqueries.ByID("pg_matviews_definitions_v1")
	if q == nil {
		t.Fatal("pg_matviews_definitions_v1 not registered")
	}
	if err := pgqueries.LintQuery(q.SQL); err != nil {
		t.Errorf("pg_matviews_definitions_v1 failed linter: %v", err)
	}
}

func TestMatviewsDefinitionsCollectorIncludesDefinition(t *testing.T) {
	q := pgqueries.ByID("pg_matviews_definitions_v1")
	if q == nil {
		t.Fatal("pg_matviews_definitions_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)

	for _, col := range []string{"schemaname", "matviewname", "matviewowner", "ispopulated", "definition"} {
		if !strings.Contains(sql, col) {
			t.Errorf("pg_matviews_definitions_v1 must include column %q", col)
		}
	}
}

// ---------------------------------------------------------------------------
// Schema Metadata Collectors — Phase 2 Step 8: pg_partitions_v1
// ---------------------------------------------------------------------------

func TestPartitionsCollectorRegistered(t *testing.T) {
	q := pgqueries.ByID("pg_partitions_v1")
	if q == nil {
		t.Fatal("pg_partitions_v1 is not registered")
	}
	if q.Category != "schema" {
		t.Errorf("category: got %q, want %q", q.Category, "schema")
	}
}

func TestPartitionsCollectorPassesLinter(t *testing.T) {
	q := pgqueries.ByID("pg_partitions_v1")
	if q == nil {
		t.Fatal("pg_partitions_v1 not registered")
	}
	if err := pgqueries.LintQuery(q.SQL); err != nil {
		t.Errorf("pg_partitions_v1 failed linter: %v", err)
	}
}

func TestPartitionsCollectorCadence(t *testing.T) {
	q := pgqueries.ByID("pg_partitions_v1")
	if q == nil {
		t.Fatal("pg_partitions_v1 not registered")
	}
	if q.Cadence != pgqueries.CadenceDaily {
		t.Errorf("cadence: got %v, want CadenceDaily (24h)", q.Cadence)
	}
}

func TestPartitionsCollectorExcludesSystemSchemas(t *testing.T) {
	q := pgqueries.ByID("pg_partitions_v1")
	if q == nil {
		t.Fatal("pg_partitions_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)
	for _, schema := range []string{"pg_catalog", "information_schema", "pg_toast", "pg_temp_"} {
		if !strings.Contains(sql, schema) {
			t.Errorf("pg_partitions_v1 must filter out %q", schema)
		}
	}
}

func TestPartitionsCollectorHasOrderBy(t *testing.T) {
	q := pgqueries.ByID("pg_partitions_v1")
	if q == nil {
		t.Fatal("pg_partitions_v1 not registered")
	}
	if !containsCI(q.SQL, "ORDER BY") {
		t.Error("pg_partitions_v1 must have ORDER BY for deterministic output")
	}
}

func TestPartitionsCollectorNoSelectStar(t *testing.T) {
	q := pgqueries.ByID("pg_partitions_v1")
	if q == nil {
		t.Fatal("pg_partitions_v1 not registered")
	}
	if strings.Contains(q.SQL, "SELECT *") || strings.Contains(q.SQL, "select *") {
		t.Error("pg_partitions_v1 must not use SELECT *")
	}
}

func TestPartitionsCollectorOutputColumns(t *testing.T) {
	q := pgqueries.ByID("pg_partitions_v1")
	if q == nil {
		t.Fatal("pg_partitions_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)

	for _, col := range []string{
		"parent_schema", "parent_name", "partition_strategy",
		"partition_key", "child_schema", "child_name", "child_bounds",
	} {
		if !strings.Contains(sql, col) {
			t.Errorf("pg_partitions_v1 must include column %q", col)
		}
	}
}

func TestPartitionsCollectorUsesPgPartitionedTable(t *testing.T) {
	q := pgqueries.ByID("pg_partitions_v1")
	if q == nil {
		t.Fatal("pg_partitions_v1 not registered")
	}
	if !containsCI(q.SQL, "pg_partitioned_table") {
		t.Error("pg_partitions_v1 must use pg_partitioned_table catalog")
	}
}

func TestPartitionsCollectorUsesPgGetPartkeydef(t *testing.T) {
	q := pgqueries.ByID("pg_partitions_v1")
	if q == nil {
		t.Fatal("pg_partitions_v1 not registered")
	}
	if !containsCI(q.SQL, "pg_get_partkeydef") {
		t.Error("pg_partitions_v1 must use pg_get_partkeydef() for partition key")
	}
}

func TestPartitionsCollectorUsesPgInherits(t *testing.T) {
	q := pgqueries.ByID("pg_partitions_v1")
	if q == nil {
		t.Fatal("pg_partitions_v1 not registered")
	}
	if !containsCI(q.SQL, "pg_inherits") {
		t.Error("pg_partitions_v1 must use pg_inherits for parent-child relationships")
	}
}

func TestPartitionsCollectorResultKind(t *testing.T) {
	q := pgqueries.ByID("pg_partitions_v1")
	if q == nil {
		t.Fatal("pg_partitions_v1 not registered")
	}
	if q.ResultKind != pgqueries.ResultRowset {
		t.Errorf("ResultKind: got %q, want rowset", q.ResultKind)
	}
}

func TestPartitionsCollectorIncludedOnPG14(t *testing.T) {
	filtered := pgqueries.Filter(pgqueries.FilterParams{
		PGMajorVersion: 14,
		Extensions:     []string{},
	})
	found := false
	for _, q := range filtered {
		if q.ID == "pg_partitions_v1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("pg_partitions_v1 must be included on PG 14")
	}
}

// ---------------------------------------------------------------------------
// Schema Metadata Collectors — Phase 2 Step 9: pg_triggers_v1
// ---------------------------------------------------------------------------

// --- pg_triggers_v1 inventory mode ---

func TestTriggersCollectorRegistered(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_v1")
	if q == nil {
		t.Fatal("pg_triggers_v1 is not registered")
	}
	if q.Category != "schema" {
		t.Errorf("category: got %q, want %q", q.Category, "schema")
	}
}

func TestTriggersCollectorPassesLinter(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_v1")
	if q == nil {
		t.Fatal("pg_triggers_v1 not registered")
	}
	if err := pgqueries.LintQuery(q.SQL); err != nil {
		t.Errorf("pg_triggers_v1 failed linter: %v", err)
	}
}

func TestTriggersCollectorCadence(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_v1")
	if q == nil {
		t.Fatal("pg_triggers_v1 not registered")
	}
	if q.Cadence != pgqueries.CadenceDaily {
		t.Errorf("cadence: got %v, want CadenceDaily (24h)", q.Cadence)
	}
}

func TestTriggersCollectorExcludesSystemSchemas(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_v1")
	if q == nil {
		t.Fatal("pg_triggers_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)
	for _, schema := range []string{"pg_catalog", "information_schema", "pg_toast", "pg_temp_"} {
		if !strings.Contains(sql, schema) {
			t.Errorf("pg_triggers_v1 must filter out %q", schema)
		}
	}
}

func TestTriggersCollectorHasOrderBy(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_v1")
	if q == nil {
		t.Fatal("pg_triggers_v1 not registered")
	}
	if !containsCI(q.SQL, "ORDER BY") {
		t.Error("pg_triggers_v1 must have ORDER BY for deterministic output")
	}
}

func TestTriggersCollectorNoSelectStar(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_v1")
	if q == nil {
		t.Fatal("pg_triggers_v1 not registered")
	}
	if strings.Contains(q.SQL, "SELECT *") || strings.Contains(q.SQL, "select *") {
		t.Error("pg_triggers_v1 must not use SELECT *")
	}
}

func TestTriggersCollectorInventoryColumns(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_v1")
	if q == nil {
		t.Fatal("pg_triggers_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)

	for _, col := range []string{
		"schemaname", "relname", "tgname", "tgtype",
		"tg_funcschema", "tg_funcname", "tg_enabled",
	} {
		if !strings.Contains(sql, col) {
			t.Errorf("pg_triggers_v1 must include column %q", col)
		}
	}
}

func TestTriggersCollectorExcludesInternalTriggers(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_v1")
	if q == nil {
		t.Fatal("pg_triggers_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)
	if !strings.Contains(sql, "tgisinternal") {
		t.Error("pg_triggers_v1 must exclude internal triggers (tgisinternal)")
	}
}

func TestTriggersCollectorEmitsTgtype(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_v1")
	if q == nil {
		t.Fatal("pg_triggers_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)
	// Must emit tgtype as integer for analyzer-side decoding
	if !strings.Contains(sql, "tgtype") {
		t.Error("pg_triggers_v1 must emit tgtype bitmask")
	}
	if strings.Contains(sql, "pg_get_triggerdef") {
		t.Error("inventory mode must not use pg_get_triggerdef")
	}
}

func TestTriggersCollectorUsesPgTrigger(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_v1")
	if q == nil {
		t.Fatal("pg_triggers_v1 not registered")
	}
	if !containsCI(q.SQL, "pg_trigger") {
		t.Error("pg_triggers_v1 must use pg_trigger catalog")
	}
}

func TestTriggersCollectorResultKind(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_v1")
	if q == nil {
		t.Fatal("pg_triggers_v1 not registered")
	}
	if q.ResultKind != pgqueries.ResultRowset {
		t.Errorf("ResultKind: got %q, want rowset", q.ResultKind)
	}
}

func TestTriggersCollectorIncludedOnPG14(t *testing.T) {
	filtered := pgqueries.Filter(pgqueries.FilterParams{
		PGMajorVersion: 14,
		Extensions:     []string{},
	})
	found := false
	for _, q := range filtered {
		if q.ID == "pg_triggers_v1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("pg_triggers_v1 must be included on PG 14")
	}
}

// --- pg_triggers_definitions_v1 definition mode ---

func TestTriggersDefinitionsCollectorRegistered(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_definitions_v1")
	if q == nil {
		t.Fatal("pg_triggers_definitions_v1 is not registered")
	}
	if q.Category != "schema" {
		t.Errorf("category: got %q, want %q", q.Category, "schema")
	}
}

func TestTriggersDefinitionsCollectorPassesLinter(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_definitions_v1")
	if q == nil {
		t.Fatal("pg_triggers_definitions_v1 not registered")
	}
	if err := pgqueries.LintQuery(q.SQL); err != nil {
		t.Errorf("pg_triggers_definitions_v1 failed linter: %v", err)
	}
}

func TestTriggersDefinitionsCollectorIncludesTriggerdef(t *testing.T) {
	q := pgqueries.ByID("pg_triggers_definitions_v1")
	if q == nil {
		t.Fatal("pg_triggers_definitions_v1 not registered")
	}
	sql := strings.ToLower(q.SQL)

	for _, col := range []string{"schemaname", "relname", "tgname", "triggerdef"} {
		if !strings.Contains(sql, col) {
			t.Errorf("pg_triggers_definitions_v1 must include column %q", col)
		}
	}
	if !strings.Contains(sql, "pg_get_triggerdef") {
		t.Error("pg_triggers_definitions_v1 must use pg_get_triggerdef()")
	}
}

// --- Updated catalog count ---

func TestSchemaPhase2TriggersCatalogCount(t *testing.T) {
	all := pgqueries.All()
	// 29 existing + 5 Phase 1/2a + 2 views + 2 matviews + 1 partitions + 2 triggers = 41
	if len(all) < 41 {
		t.Errorf("catalog has %d collectors, want at least 41", len(all))
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
