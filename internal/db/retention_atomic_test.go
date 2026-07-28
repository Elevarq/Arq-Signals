// Copyright (c) 2026 Scantr LLC. All rights reserved.
// Elevarq is a trade name of Scantr LLC.
// This file is part of Elevarq Signals. Use is governed by the
// commercial license at LICENSE in the repository root.

package db

import (
	"errors"
	"testing"
)

// Issue #327 / FC-23 / INV-SNAP-STATUS-PAYLOAD (retention producer):
// retention cleanup MUST delete a run's query_results payload and its
// query_runs row atomically. A failure after the payload delete but
// before the run delete must roll BOTH back, so retention can never
// leave a status=success run without its joinable payload (the #312
// orphaned-success failure reached from the retention path).
//
// These tests inject a failure between the two deletes via the
// package-private afterResultsDeleteHook and assert nothing was
// removed, then assert the post-cleanup invariant on a real prune.

// newRetentionTestDB opens an in-memory DB, migrates it, and seeds a
// single target so query_runs' FK to targets is satisfied.
func newRetentionTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(":memory:", false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := d.sql.Exec(
		`INSERT INTO targets(id, name, created_at, updated_at)
		 VALUES (1, 't1', '2020-01-01T00:00:00Z', '2020-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	return d
}

// seedRun inserts a success query_run + its query_results payload, and
// registers the query in the catalog with the given retention class.
func seedRun(t *testing.T, d *DB, runID, queryID, class, collectedAt string, rowCount int) {
	t.Helper()
	if _, err := d.sql.Exec(
		`INSERT OR IGNORE INTO query_catalog(query_id, retention_class, registered_at)
		 VALUES (?, ?, '2020-01-01T00:00:00Z')`, queryID, class); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	if _, err := d.sql.Exec(
		`INSERT INTO query_runs(id, target_id, query_id, collected_at, row_count, created_at)
		 VALUES (?, 1, ?, ?, ?, ?)`, runID, queryID, collectedAt, rowCount, collectedAt); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if _, err := d.sql.Exec(
		`INSERT INTO query_results(run_id, payload, compressed, size_bytes)
		 VALUES (?, ?, 0, ?)`, runID, []byte(`{"x":1}`), 7); err != nil {
		t.Fatalf("seed result: %v", err)
	}
}

func countRuns(t *testing.T, d *DB) int {
	t.Helper()
	var n int
	if err := d.sql.QueryRow("SELECT COUNT(*) FROM query_runs").Scan(&n); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	return n
}

func countResults(t *testing.T, d *DB) int {
	t.Helper()
	var n int
	if err := d.sql.QueryRow("SELECT COUNT(*) FROM query_results").Scan(&n); err != nil {
		t.Fatalf("count results: %v", err)
	}
	return n
}

// assertInvariant fails if any success run lacks exactly one joinable
// payload (INV-SNAP-STATUS-PAYLOAD). row_count here is a proxy for the
// success runs the collector records; every remaining run must join.
func assertInvariant(t *testing.T, d *DB) {
	t.Helper()
	var orphans int
	if err := d.sql.QueryRow(
		`SELECT COUNT(*) FROM query_runs qr
		 WHERE NOT EXISTS (SELECT 1 FROM query_results res WHERE res.run_id = qr.id)`,
	).Scan(&orphans); err != nil {
		t.Fatalf("orphan check: %v", err)
	}
	if orphans != 0 {
		t.Fatalf("INV-SNAP-STATUS-PAYLOAD violated: %d run(s) have no joinable payload", orphans)
	}
}

func TestDeleteQueryRunsOlderThan_AtomicOnSecondStepFailure(t *testing.T) {
	d := newRetentionTestDB(t)
	// One old run that the cutoff would prune.
	seedRun(t, d, "run-old", "q1", "short", "2020-01-01T00:00:00Z", 3)

	injected := errors.New("injected failure between deletes")
	d.afterResultsDeleteHook = func() error { return injected }
	defer func() { d.afterResultsDeleteHook = nil }()

	_, err := d.DeleteQueryRunsOlderThan("2099-01-01T00:00:00Z")
	if err == nil {
		t.Fatal("expected the injected failure to surface, got nil")
	}
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got %v", err)
	}

	// The transaction must have rolled BOTH deletes back.
	if got := countResults(t, d); got != 1 {
		t.Fatalf("expected payload to survive rollback, got %d query_results rows", got)
	}
	if got := countRuns(t, d); got != 1 {
		t.Fatalf("expected run to survive rollback, got %d query_runs rows", got)
	}
	assertInvariant(t, d)
}

func TestDeleteQueryRunsOlderThanByClass_AtomicOnSecondStepFailure(t *testing.T) {
	d := newRetentionTestDB(t)
	seedRun(t, d, "run-old", "q1", "short", "2020-01-01T00:00:00Z", 5)

	injected := errors.New("injected failure between deletes")
	d.afterResultsDeleteHook = func() error { return injected }
	defer func() { d.afterResultsDeleteHook = nil }()

	_, err := d.DeleteQueryRunsOlderThanByClass("short", "2099-01-01T00:00:00Z")
	if err == nil {
		t.Fatal("expected the injected failure to surface, got nil")
	}
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got %v", err)
	}

	if got := countResults(t, d); got != 1 {
		t.Fatalf("expected payload to survive rollback, got %d query_results rows", got)
	}
	if got := countRuns(t, d); got != 1 {
		t.Fatalf("expected run to survive rollback, got %d query_runs rows", got)
	}
	assertInvariant(t, d)
}

// After a real prune (no injected failure) the invariant must hold:
// old runs and their payloads are both gone, and every remaining
// success run still has exactly one joinable payload.
func TestDeleteQueryRunsOlderThan_InvariantHoldsAfterCleanup(t *testing.T) {
	d := newRetentionTestDB(t)
	seedRun(t, d, "run-old", "q1", "short", "2020-01-01T00:00:00Z", 2)
	seedRun(t, d, "run-new", "q1", "short", "2099-06-01T00:00:00Z", 4)

	deleted, err := d.DeleteQueryRunsOlderThan("2099-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 run deleted, got %d", deleted)
	}
	// Old run + its payload gone; new run + payload intact.
	if got := countRuns(t, d); got != 1 {
		t.Fatalf("expected 1 remaining run, got %d", got)
	}
	if got := countResults(t, d); got != 1 {
		t.Fatalf("expected 1 remaining payload, got %d", got)
	}
	assertInvariant(t, d)
}

func TestDeleteQueryRunsOlderThanByClass_InvariantHoldsAfterCleanup(t *testing.T) {
	d := newRetentionTestDB(t)
	// short-class old run should be pruned; long-class old run must survive.
	seedRun(t, d, "run-short-old", "q-short", "short", "2020-01-01T00:00:00Z", 2)
	seedRun(t, d, "run-long-old", "q-long", "long", "2020-01-01T00:00:00Z", 6)

	deleted, err := d.DeleteQueryRunsOlderThanByClass("short", "2099-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 short-class run deleted, got %d", deleted)
	}
	if got := countRuns(t, d); got != 1 {
		t.Fatalf("expected the long-class run to survive, got %d runs", got)
	}
	if got := countResults(t, d); got != 1 {
		t.Fatalf("expected the long-class payload to survive, got %d payloads", got)
	}
	assertInvariant(t, d)
}
