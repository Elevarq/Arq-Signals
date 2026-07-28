// Tests for R110 concurrency safety (issue #323): a single long-lived
// export.Builder is shared across concurrent GET /export requests (as
// the daemon does), so the request-specific scope of one WriteTo call
// must be local and immutable for that call. Two concurrent WriteTo
// calls with different selectors must each produce an archive that
// contains ONLY its own scope, and must be clean under `go test -race`.
//
// These tests exercise the PRODUCTION shared-Builder path: one Builder,
// many concurrent WriteTo calls.

package tests

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elevarq/signals/internal/db"
	"github.com/elevarq/signals/internal/export"
)

// seedTwoSnapshotFixture inserts one target with two distinguishable
// snapshots, each carrying a run for a distinct collector so a
// snapshot-scoped export and an all-history export are trivially
// distinguishable by their query_runs.ndjson contents.
//
// Returns the two snapshot IDs and the query_ids that identify each.
func seedTwoSnapshotFixture(t *testing.T, store *db.DB) (snapA, snapB, qidA, qidB string, targetID int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	var err error
	targetID, err = store.UpsertTarget("target-A", "hostA", 5432, "dbA", "u", "disable", "NONE", "", true)
	if err != nil {
		t.Fatalf("UpsertTarget: %v", err)
	}

	snapA, snapB = "snap-A", "snap-B"
	qidA, qidB = "collector_a_v1", "collector_b_v1"

	for _, s := range []db.Snapshot{
		{ID: snapA, TargetID: targetID, CollectedAt: "2026-01-01T00:00:00Z", PGVersion: "16", Payload: []byte(`{"which":"A"}`), SizeBytes: 13},
		{ID: snapB, TargetID: targetID, CollectedAt: "2026-02-01T00:00:00Z", PGVersion: "16", Payload: []byte(`{"which":"B"}`), SizeBytes: 13},
	} {
		if err := store.InsertSnapshot(s); err != nil {
			t.Fatalf("InsertSnapshot %s: %v", s.ID, err)
		}
	}

	rowsA := []map[string]any{{"which": "A"}}
	payA, compA, sizeA, _ := db.EncodeNDJSON(rowsA)
	rowsB := []map[string]any{{"which": "B"}}
	payB, compB, sizeB, _ := db.EncodeNDJSON(rowsB)

	runs := []db.QueryRun{
		{ID: "run-A", TargetID: targetID, SnapshotID: snapA, QueryID: qidA, CollectedAt: "2026-01-01T00:00:00Z", PGVersion: "16", RowCount: 1, Status: "success", CreatedAt: now},
		{ID: "run-B", TargetID: targetID, SnapshotID: snapB, QueryID: qidB, CollectedAt: "2026-02-01T00:00:00Z", PGVersion: "16", RowCount: 1, Status: "success", CreatedAt: now},
	}
	results := []db.QueryResult{
		{RunID: "run-A", Payload: payA, Compressed: compA, SizeBytes: sizeA},
		{RunID: "run-B", Payload: payB, Compressed: compB, SizeBytes: sizeB},
	}
	if err := store.InsertQueryRunBatch(runs, results); err != nil {
		t.Fatalf("InsertQueryRunBatch: %v", err)
	}
	return snapA, snapB, qidA, qidB, targetID
}

// runIDsInArchive returns the set of run ids emitted in query_runs.ndjson.
func runIDsInArchive(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	out := map[string]bool{}
	for _, f := range zr.File {
		if f.Name != "query_runs.ndjson" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open query_runs.ndjson: %v", err)
		}
		raw, _ := io.ReadAll(rc)
		rc.Close()
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			if line == "" {
				continue
			}
			var row map[string]any
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				t.Fatalf("decode run row: %v", err)
			}
			if id, ok := row["id"].(string); ok {
				out[id] = true
			}
		}
	}
	return out
}

// TestConcurrentExportsDoNotCrossContaminate runs many concurrent
// WriteTo calls on ONE shared Builder — exactly the daemon topology —
// with two different selectors: a snapshot-scoped export (only snap-A →
// run-A) and an all-history export (run-A + run-B). Each archive must
// contain ONLY its own scope. This fails before the fix (shared mutable
// Builder fields race and cross-contaminate) and passes after (per-call
// local scope). Run under `go test -race`.
// Traces: SIGNALS-R110 (issue #323)
func TestConcurrentExportsDoNotCrossContaminate(t *testing.T) {
	store := openTestDB(t)
	snapA, _, _, _, _ := seedTwoSnapshotFixture(t, store)

	// ONE shared Builder — the production topology (cmd/signals/main.go
	// builds one and api.Deps shares it across every request).
	builder := export.NewBuilder(store, "shared-instance")

	const iterations = 40
	var wg sync.WaitGroup
	errCh := make(chan error, iterations*2)

	for i := 0; i < iterations; i++ {
		wg.Add(2)

		// Selector 1: snapshot-scoped → expect exactly {run-A}.
		go func() {
			defer wg.Done()
			var buf bytes.Buffer
			if err := builder.WriteTo(&buf, export.Options{SnapshotID: snapA}); err != nil {
				errCh <- err
				return
			}
			ids := runIDsInArchive(t, buf.Bytes())
			if !ids["run-A"] || ids["run-B"] || len(ids) != 1 {
				t.Errorf("snapshot-scoped archive leaked scope: got run ids %v, want only {run-A}", ids)
			}
		}()

		// Selector 2: all-history → expect exactly {run-A, run-B}.
		go func() {
			defer wg.Done()
			var buf bytes.Buffer
			if err := builder.WriteTo(&buf, export.Options{All: true}); err != nil {
				errCh <- err
				return
			}
			ids := runIDsInArchive(t, buf.Bytes())
			if !ids["run-A"] || !ids["run-B"] || len(ids) != 2 {
				t.Errorf("all-history archive leaked scope: got run ids %v, want {run-A, run-B}", ids)
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent WriteTo: %v", err)
	}
}
