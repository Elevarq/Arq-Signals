//go:build integration

// Deterministic regression for #383: the pg_stat_progress_* family collectors
// share one family spec (pg_stat_progress_family_v1.md) that intentionally
// carries no "## Output columns" table (their columns are per-major and
// documented inline at the registration site to avoid spec/code drift). The
// output-contract resolver (declaredColumnsForCollector) therefore cannot
// derive a fixed column list for them; each must be classified column-dynamic
// (dynamicColumnCollectors) so the resolver never looks for a non-existent
// per-collector <id>.md. Before this classification the harness Fatal'd
// whenever a progress row was sampled — a flake, because a progress row only
// exists while the operation is in flight.
//
// This test needs no live PostgreSQL, so it runs on every integration build
// and catches a newly-added progress collector deterministically, rather than
// waiting for an ANALYZE to coincide with a collection.

package tests

import (
	"strings"
	"testing"

	"github.com/elevarq/signals/internal/pgqueries"
)

func TestProgressFamilyCollectorsAreClassified_383(t *testing.T) {
	found := 0
	for _, q := range pgqueries.All() {
		if !strings.HasPrefix(q.ID, "pg_stat_progress_") {
			continue
		}
		found++
		if _, ok := dynamicColumnCollectors[q.ID]; !ok {
			t.Errorf("#383: progress-family collector %q is not classified column-dynamic. "+
				"Add it to dynamicColumnCollectors — the family spec "+
				"pg_stat_progress_family_v1.md has no per-collector column table, so the "+
				"output-contract resolver cannot derive a declared-column list for it.", q.ID)
		}
	}
	if found == 0 {
		t.Fatal("#383: no pg_stat_progress_* collectors found in pgqueries.All() — " +
			"did the registry or the naming convention change? Update this regression.")
	}
}
