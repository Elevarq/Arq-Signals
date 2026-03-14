package tests

import (
	"sort"
	"testing"

	"github.com/elevarq/arq-signals/internal/pgqueries"
)

// TestCatalogMinimumCount verifies that the query catalog contains at
// least 9 registered queries.
// Traces: ARQ-SIGNALS-R002 / TC-SIG-004
func TestCatalogMinimumCount(t *testing.T) {
	all := pgqueries.All()
	if len(all) < 9 {
		t.Fatalf("expected >= 9 registered queries, got %d", len(all))
	}
}

// TestCatalogRequiredQueries verifies all 9 required query IDs are
// present in the catalog.
// Traces: ARQ-SIGNALS-R003 / TC-SIG-005
func TestCatalogRequiredQueries(t *testing.T) {
	required := []string{
		"pg_version_v1",
		"pg_settings_v1",
		"pg_stat_activity_v1",
		"pg_stat_database_v1",
		"pg_stat_user_tables_v1",
		"pg_stat_user_indexes_v1",
		"pg_statio_user_tables_v1",
		"pg_statio_user_indexes_v1",
		"pg_stat_statements_v1",
	}

	all := pgqueries.All()
	idSet := make(map[string]bool, len(all))
	for _, q := range all {
		idSet[q.ID] = true
	}

	for _, id := range required {
		if !idSet[id] {
			t.Errorf("required query %q is missing from the catalog", id)
		}
	}
}

// TestCatalogAllPassLint verifies every registered query passes the linter.
// Traces: ARQ-SIGNALS-R002 / TC-SIG-004
func TestCatalogAllPassLint(t *testing.T) {
	for _, q := range pgqueries.All() {
		if err := pgqueries.LintQuery(q.SQL); err != nil {
			t.Errorf("query %q failed lint: %v", q.ID, err)
		}
	}
}

// TestCatalogSorted verifies that All() returns queries sorted by ID.
// Traces: ARQ-SIGNALS-R002 / TC-SIG-004
func TestCatalogSorted(t *testing.T) {
	all := pgqueries.All()
	ids := make([]string, len(all))
	for i, q := range all {
		ids[i] = q.ID
	}
	if !sort.StringsAreSorted(ids) {
		t.Errorf("All() result is not sorted by ID: %v", ids)
	}
}
