package tests

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elevarq/signals/internal/db"
	"github.com/elevarq/signals/internal/export"
)

// buildExportZIPWithBuilder runs an arbitrary configured Builder to a
// buffer and returns the parsed reader. Variant of buildExportZIP that
// lets the caller toggle SetExportPerCollectorFiles before WriteTo.
func buildExportZIPWithBuilder(t *testing.T, b *export.Builder) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	if err := b.WriteTo(&buf, export.Options{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	return zr
}

// listPerCollectorFiles returns the names of files inside per-collector/.
func listPerCollectorFiles(zr *zip.Reader) []string {
	var out []string
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "per-collector/") {
			out = append(out, f.Name)
		}
	}
	return out
}

// readZipFile returns the raw bytes of a named ZIP entry.
func readZipFile(t *testing.T, zr *zip.Reader, name string) []byte {
	t.Helper()
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open %s: %v", name, err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			return data
		}
	}
	t.Fatalf("entry %q not in ZIP", name)
	return nil
}

// ---------------------------------------------------------------------------
// R080: Per-collector export view
// ---------------------------------------------------------------------------

// TestPerCollectorExportOffByDefault verifies the safe default: no
// per-collector/ directory unless the operator explicitly opts in.
// Traces: SIGNALS-R080
func TestPerCollectorExportOffByDefault(t *testing.T) {
	store := openTestDB(t)
	seedExportData(t, store)

	builder := export.NewBuilder(store, "test-instance-id")
	zr := buildExportZIPWithBuilder(t, builder)

	if names := listPerCollectorFiles(zr); len(names) != 0 {
		t.Errorf("per-collector files leaked into default export: %v", names)
	}
}

// TestPerCollectorExportEnabled verifies that with the flag on, every
// collector that produced a query_run for this scope gets exactly one
// per-collector/<query_id>.json file.
// Traces: SIGNALS-R080
func TestPerCollectorExportEnabled(t *testing.T) {
	store := openTestDB(t)
	seedExportData(t, store)

	builder := export.NewBuilder(store, "test-instance-id")
	builder.SetExportPerCollectorFiles(true)
	zr := buildExportZIPWithBuilder(t, builder)

	names := listPerCollectorFiles(zr)
	if len(names) != 1 {
		t.Fatalf("expected 1 per-collector file (one collector ran), got %d: %v", len(names), names)
	}
	// Grouped by (target_id, query_id): file lives under the target
	// subdirectory (issue #322 attribution).
	if !strings.HasSuffix(names[0], "/pg_settings_v1.json") ||
		!strings.HasPrefix(names[0], "per-collector/") {
		t.Errorf("unexpected file name: %q (want per-collector/<target_id>/pg_settings_v1.json)", names[0])
	}

	// Content shape: latest run metadata + payload rows.
	raw := readZipFile(t, zr, names[0])
	var entry map[string]any
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatalf("decode per-collector entry: %v", err)
	}
	if got, want := entry["query_id"], "pg_settings_v1"; got != want {
		t.Errorf("query_id = %v, want %q", got, want)
	}
	if got, want := entry["status"], "success"; got != want {
		t.Errorf("status = %v, want %q", got, want)
	}
	rows, ok := entry["rows"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("rows missing or wrong shape: %v", entry["rows"])
	}
	row := rows[0].(map[string]any)
	if row["setting"] != "shared_buffers" || row["value"] != "128MB" {
		t.Errorf("row payload mismatch: %v", row)
	}
}

// TestPerCollectorExportSkippedRunHasStub verifies that a skipped run
// (R075 high-sensitivity gate) still produces a per-collector entry —
// auditors browsing per-collector/ should see the gate is active. The
// entry carries status=skipped and reason=config_disabled but no
// payload rows.
// Traces: SIGNALS-R080 / SIGNALS-R075
func TestPerCollectorExportSkippedRunHasStub(t *testing.T) {
	store := openTestDB(t)

	// Minimal fixture: one target plus a single skipped run for a
	// high-sensitivity collector.
	now := time.Now().UTC().Format(time.RFC3339)
	targetID, err := store.UpsertTarget("test-pg", "h", 5432, "d", "u", "disable", "NONE", "", true)
	if err != nil {
		t.Fatalf("UpsertTarget: %v", err)
	}
	skipRun := db.QueryRun{
		ID:          "run-skip-1",
		TargetID:    targetID,
		SnapshotID:  "snap-skip",
		QueryID:     "pg_views_definitions_v1",
		CollectedAt: now,
		PGVersion:   "16",
		CreatedAt:   now,
		Status:      "skipped",
		Reason:      "config_disabled",
	}
	if err := store.InsertQueryRunBatch([]db.QueryRun{skipRun}, nil); err != nil {
		t.Fatalf("InsertQueryRunBatch: %v", err)
	}

	builder := export.NewBuilder(store, "test-instance-id")
	builder.SetExportPerCollectorFiles(true)
	zr := buildExportZIPWithBuilder(t, builder)

	raw := readZipFile(t, zr, "per-collector/"+tgtDir(targetID)+"/pg_views_definitions_v1.json")
	var entry map[string]any
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatalf("decode skipped entry: %v", err)
	}
	if entry["status"] != "skipped" {
		t.Errorf("status = %v, want skipped", entry["status"])
	}
	if entry["reason"] != "config_disabled" {
		t.Errorf("reason = %v, want config_disabled", entry["reason"])
	}
	if _, has := entry["rows"]; has {
		t.Error("skipped run should not carry a row payload")
	}
}

// TestPerCollectorExportLatestRunWins verifies the latest-run-wins
// rule: when the export covers multiple cycles for the same collector,
// the per-collector file reflects the most recent run only.
// Traces: SIGNALS-R080
func TestPerCollectorExportLatestRunWins(t *testing.T) {
	store := openTestDB(t)
	targetID, err := store.UpsertTarget("test-pg", "h", 5432, "d", "u", "disable", "NONE", "", true)
	if err != nil {
		t.Fatalf("UpsertTarget: %v", err)
	}

	// Older run.
	oldRows := []map[string]any{{"value": "old"}}
	oldPayload, oldComp, oldSize, _ := db.EncodeNDJSON(oldRows)
	older := db.QueryRun{
		ID: "run-old", TargetID: targetID, SnapshotID: "s1",
		QueryID: "demo_v1", CollectedAt: "2026-04-01T00:00:00Z",
		PGVersion: "16", DurationMS: 1, RowCount: 1, Status: "success",
		CreatedAt: "2026-04-01T00:00:00Z",
	}
	// Newer run for the same collector.
	newRows := []map[string]any{{"value": "new"}}
	newPayload, newComp, newSize, _ := db.EncodeNDJSON(newRows)
	newer := db.QueryRun{
		ID: "run-new", TargetID: targetID, SnapshotID: "s2",
		QueryID: "demo_v1", CollectedAt: "2026-04-15T00:00:00Z",
		PGVersion: "16", DurationMS: 2, RowCount: 1, Status: "success",
		CreatedAt: "2026-04-15T00:00:00Z",
	}
	results := []db.QueryResult{
		{RunID: older.ID, Payload: oldPayload, Compressed: oldComp, SizeBytes: oldSize},
		{RunID: newer.ID, Payload: newPayload, Compressed: newComp, SizeBytes: newSize},
	}
	if err := store.InsertQueryRunBatch([]db.QueryRun{older, newer}, results); err != nil {
		t.Fatalf("InsertQueryRunBatch: %v", err)
	}

	builder := export.NewBuilder(store, "test-instance-id")
	builder.SetExportPerCollectorFiles(true)
	zr := buildExportZIPWithBuilder(t, builder)

	// Grouping is by (target_id, query_id): the file lives under the
	// target subdirectory (issue #322 attribution).
	raw := readZipFile(t, zr, "per-collector/"+tgtDir(targetID)+"/demo_v1.json")
	var entry map[string]any
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatalf("decode entry: %v", err)
	}
	if entry["collected_at"] != "2026-04-15T00:00:00Z" {
		t.Errorf("expected newest collected_at, got %v", entry["collected_at"])
	}
	rows := entry["rows"].([]any)
	if rows[0].(map[string]any)["value"] != "new" {
		t.Errorf("payload mismatch: expected new, got %v", rows[0])
	}
}

// ---------------------------------------------------------------------------
// R080 scope-fidelity tests (issue #322): the per-collector directory
// is a STRICT regrouping of the resolved in-scope run set. Every
// per-collector run must also appear in canonical query_runs.ndjson,
// and scope selectors must match the canonical files exactly.
// ---------------------------------------------------------------------------

// tgtDir builds the per-collector target subdirectory path segment for
// a target id.
func tgtDir(n int64) string {
	return strconv.FormatInt(n, 10)
}

// canonicalRunIDs returns the set of run ids from query_runs.ndjson.
func canonicalRunIDs(t *testing.T, zr *zip.Reader) map[string]bool {
	t.Helper()
	raw := readZipFile(t, zr, "query_runs.ndjson")
	out := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("decode canonical run: %v", err)
		}
		if id, ok := row["id"].(string); ok {
			out[id] = true
		}
	}
	return out
}

// perCollectorRunIDs returns the run_id embedded in every per-collector
// file. Each file must carry the run_id it was regrouped from so the
// canonical cross-check (issue #322) is possible.
func perCollectorRunIDs(t *testing.T, zr *zip.Reader) []string {
	t.Helper()
	var out []string
	for _, name := range listPerCollectorFiles(zr) {
		var entry map[string]any
		if err := json.Unmarshal(readZipFile(t, zr, name), &entry); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		id, ok := entry["run_id"].(string)
		if !ok || id == "" {
			t.Fatalf("per-collector file %s missing run_id (needed for canonical cross-check)", name)
		}
		out = append(out, id)
	}
	return out
}

// assertPerCollectorSubsetOfCanonical asserts every per-collector run_id
// is present in canonical query_runs.ndjson (the core issue #322
// invariant: no per-collector payload outside the resolved scope).
func assertPerCollectorSubsetOfCanonical(t *testing.T, zr *zip.Reader) {
	t.Helper()
	canonical := canonicalRunIDs(t, zr)
	for _, id := range perCollectorRunIDs(t, zr) {
		if !canonical[id] {
			t.Errorf("per-collector run_id %q is NOT in canonical query_runs.ndjson (scope leak)", id)
		}
	}
}

// buildScopedExportZIP builds an export with the given options and
// per-collector files enabled, returning the parsed reader.
func buildScopedPerCollectorZIP(t *testing.T, store *db.DB, opts export.Options) *zip.Reader {
	t.Helper()
	b := export.NewBuilder(store, "test-instance-id")
	b.SetExportPerCollectorFiles(true)
	var buf bytes.Buffer
	if err := b.WriteTo(&buf, opts); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	return zr
}

// seedMultiTargetHistory inserts two enabled targets and one disabled
// target, each with two cycles of runs for the same collector, so scope
// selectors (snapshot, target, all, since/until, default, disabled) are
// all distinguishable. Returns the two enabled target ids, the disabled
// target id, and the snapshot ids for target1.
func seedMultiTargetHistory(t *testing.T, store *db.DB) (t1, t2, tDisabled int64, snap1Old, snap1New string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	var err error
	t1, err = store.UpsertTarget("t1", "h1", 5432, "d", "u", "disable", "NONE", "", true)
	if err != nil {
		t.Fatalf("UpsertTarget t1: %v", err)
	}
	t2, err = store.UpsertTarget("t2", "h2", 5432, "d", "u", "disable", "NONE", "", true)
	if err != nil {
		t.Fatalf("UpsertTarget t2: %v", err)
	}
	tDisabled, err = store.UpsertTarget("t3-disabled", "h3", 5432, "d", "u", "disable", "NONE", "", false)
	if err != nil {
		t.Fatalf("UpsertTarget t3: %v", err)
	}

	snap1Old, snap1New = "snap-t1-old", "snap-t1-new"
	type snapSpec struct {
		id     string
		target int64
		at     string
	}
	snaps := []snapSpec{
		{snap1Old, t1, "2026-01-01T00:00:00Z"},
		{snap1New, t1, "2026-03-01T00:00:00Z"},
		{"snap-t2", t2, "2026-02-01T00:00:00Z"},
		{"snap-t3", tDisabled, "2026-02-15T00:00:00Z"},
	}
	for _, s := range snaps {
		if err := store.InsertSnapshot(db.Snapshot{
			ID: s.id, TargetID: s.target, CollectedAt: s.at,
			PGVersion: "16", Payload: []byte(`{}`), SizeBytes: 2,
		}); err != nil {
			t.Fatalf("InsertSnapshot %s: %v", s.id, err)
		}
	}

	mkRun := func(id string, target int64, snap, at, val string) (db.QueryRun, db.QueryResult) {
		rows := []map[string]any{{"v": val}}
		p, c, sz, _ := db.EncodeNDJSON(rows)
		return db.QueryRun{
				ID: id, TargetID: target, SnapshotID: snap, QueryID: "demo_v1",
				CollectedAt: at, PGVersion: "16", RowCount: 1, Status: "success", CreatedAt: now,
			},
			db.QueryResult{RunID: id, Payload: p, Compressed: c, SizeBytes: sz}
	}

	var runs []db.QueryRun
	var results []db.QueryResult
	for _, spec := range []struct {
		id, snap, at, val string
		target            int64
	}{
		{"r-t1-old", snap1Old, "2026-01-01T00:00:00Z", "t1-old", t1},
		{"r-t1-new", snap1New, "2026-03-01T00:00:00Z", "t1-new", t1},
		{"r-t2", "snap-t2", "2026-02-01T00:00:00Z", "t2", t2},
		{"r-t3", "snap-t3", "2026-02-15T00:00:00Z", "t3", tDisabled},
	} {
		run, res := mkRun(spec.id, spec.target, spec.snap, spec.at, spec.val)
		runs = append(runs, run)
		results = append(results, res)
	}
	if err := store.InsertQueryRunBatch(runs, results); err != nil {
		t.Fatalf("InsertQueryRunBatch: %v", err)
	}
	return t1, t2, tDisabled, snap1Old, snap1New
}

// TestPerCollectorSnapshotScope: a snapshot-scoped export's
// per-collector files contain ONLY the run from that snapshot, and
// every per-collector run appears in canonical query_runs.ndjson.
// Traces: SIGNALS-R080 (issue #322)
func TestPerCollectorSnapshotScope(t *testing.T) {
	store := openTestDB(t)
	t1, _, _, snap1Old, _ := seedMultiTargetHistory(t, store)

	zr := buildScopedPerCollectorZIP(t, store, export.Options{SnapshotID: snap1Old})
	assertPerCollectorSubsetOfCanonical(t, zr)

	ids := perCollectorRunIDs(t, zr)
	if len(ids) != 1 || ids[0] != "r-t1-old" {
		t.Fatalf("snapshot scope: per-collector run ids = %v, want [r-t1-old]", ids)
	}
	// The old snapshot's run must be the one exported, not the newer one.
	if _, err := zr.Open("per-collector/" + tgtDir(t1) + "/demo_v1.json"); err != nil {
		t.Fatalf("expected per-collector/%d/demo_v1.json: %v", t1, err)
	}
}

// TestPerCollectorTargetScope: a target-scoped export contains only
// that target's latest run and nothing from other targets.
// Traces: SIGNALS-R080 (issue #322)
func TestPerCollectorTargetScope(t *testing.T) {
	store := openTestDB(t)
	_, t2, _, _, _ := seedMultiTargetHistory(t, store)

	zr := buildScopedPerCollectorZIP(t, store, export.Options{TargetID: t2, All: true})
	assertPerCollectorSubsetOfCanonical(t, zr)

	ids := perCollectorRunIDs(t, zr)
	if len(ids) != 1 || ids[0] != "r-t2" {
		t.Fatalf("target scope: per-collector run ids = %v, want [r-t2]", ids)
	}
}

// TestPerCollectorAllScopeMultiTargetAttribution: --all across two
// targets keeps BOTH targets' latest run as distinct files under their
// own target subdirectory — no target silently wins a query_id-only
// grouping.
// Traces: SIGNALS-R080 (issue #322)
func TestPerCollectorAllScopeMultiTargetAttribution(t *testing.T) {
	store := openTestDB(t)
	t1, t2, tDisabled, _, _ := seedMultiTargetHistory(t, store)

	zr := buildScopedPerCollectorZIP(t, store, export.Options{All: true})
	assertPerCollectorSubsetOfCanonical(t, zr)

	names := listPerCollectorFiles(zr)
	// --all is full history INCLUDING disabled targets (only the R084
	// default excludes them). Each of the three targets keeps its own
	// (target_id, query_id) file — attribution is preserved, no target
	// silently wins a query_id-only grouping.
	want := map[string]bool{
		"per-collector/" + tgtDir(t1) + "/demo_v1.json":        true,
		"per-collector/" + tgtDir(t2) + "/demo_v1.json":        true,
		"per-collector/" + tgtDir(tDisabled) + "/demo_v1.json": true,
	}
	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	for w := range want {
		if !got[w] {
			t.Errorf("multi-target: missing %s (attribution collapsed); got %v", w, names)
		}
	}
	// t1 has two runs but latest-run-wins keeps one file per (target,
	// query). Three distinct targets → three files.
	if len(names) != 3 {
		t.Fatalf("multi-target --all: expected 3 per-collector files, got %d: %v", len(names), names)
	}
	// Latest-run-wins for t1: the exported run is the newer one.
	var entry map[string]any
	if err := json.Unmarshal(readZipFile(t, zr, "per-collector/"+tgtDir(t1)+"/demo_v1.json"), &entry); err != nil {
		t.Fatalf("decode t1 entry: %v", err)
	}
	if entry["run_id"] != "r-t1-new" {
		t.Errorf("t1 per-collector run_id = %v, want r-t1-new (latest-run-wins)", entry["run_id"])
	}
}

// TestPerCollectorSinceUntilScope: a time-window export includes only
// the runs inside the window, matching the canonical files.
// Traces: SIGNALS-R080 (issue #322)
func TestPerCollectorSinceUntilScope(t *testing.T) {
	store := openTestDB(t)
	seedMultiTargetHistory(t, store)

	// Window covers only Feb → only t2's run (2026-02-01) qualifies.
	zr := buildScopedPerCollectorZIP(t, store, export.Options{
		Since: "2026-01-15T00:00:00Z",
		Until: "2026-02-10T00:00:00Z",
	})
	assertPerCollectorSubsetOfCanonical(t, zr)

	ids := perCollectorRunIDs(t, zr)
	if len(ids) != 1 || ids[0] != "r-t2" {
		t.Fatalf("since/until scope: per-collector run ids = %v, want [r-t2]", ids)
	}
}

// TestPerCollectorDefaultActiveTargetExcludesDisabled: the R084 default
// scope (no selectors) exports the latest run per collector for ACTIVE
// targets only — the disabled target's run must not appear in the
// per-collector directory (nor in canonical runs).
// Traces: SIGNALS-R080 (issue #322)
func TestPerCollectorDefaultActiveTargetExcludesDisabled(t *testing.T) {
	store := openTestDB(t)
	seedMultiTargetHistory(t, store)

	zr := buildScopedPerCollectorZIP(t, store, export.Options{})
	assertPerCollectorSubsetOfCanonical(t, zr)

	for _, id := range perCollectorRunIDs(t, zr) {
		if id == "r-t3" {
			t.Errorf("default scope leaked disabled-target run r-t3 into per-collector files")
		}
	}
	// Default keeps t1's LATEST run (r-t1-new), not the old one.
	ids := map[string]bool{}
	for _, id := range perCollectorRunIDs(t, zr) {
		ids[id] = true
	}
	if ids["r-t1-old"] {
		t.Errorf("default scope should carry t1's latest run, not r-t1-old")
	}
	if !ids["r-t1-new"] || !ids["r-t2"] {
		t.Errorf("default scope missing expected latest runs: got %v", ids)
	}
}

// TestPerCollectorMissingPayloadFailsExport: a successful in-scope run
// whose result payload is missing must FAIL the export (issue #322 —
// no zero-row success stub), consistent with query_results.ndjson.
// Traces: SIGNALS-R080 (issue #322)
func TestPerCollectorMissingPayloadFailsExport(t *testing.T) {
	store := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	targetID, err := store.UpsertTarget("t", "h", 5432, "d", "u", "disable", "NONE", "", true)
	if err != nil {
		t.Fatalf("UpsertTarget: %v", err)
	}
	if err := store.InsertSnapshot(db.Snapshot{
		ID: "snap-x", TargetID: targetID, CollectedAt: now,
		PGVersion: "16", Payload: []byte(`{}`), SizeBytes: 2,
	}); err != nil {
		t.Fatalf("InsertSnapshot: %v", err)
	}
	// Successful run with NO accompanying result payload (nil results).
	run := db.QueryRun{
		ID: "run-nopayload", TargetID: targetID, SnapshotID: "snap-x",
		QueryID: "demo_v1", CollectedAt: now, PGVersion: "16",
		RowCount: 1, Status: "success", CreatedAt: now,
	}
	if err := store.InsertQueryRunBatch([]db.QueryRun{run}, nil); err != nil {
		t.Fatalf("InsertQueryRunBatch: %v", err)
	}

	b := export.NewBuilder(store, "test-instance-id")
	b.SetExportPerCollectorFiles(true)
	var buf bytes.Buffer
	err = b.WriteTo(&buf, export.Options{All: true})
	if err == nil {
		t.Fatal("expected export to FAIL on missing payload for successful in-scope run, got nil (success stub)")
	}
	if !strings.Contains(err.Error(), "run-nopayload") {
		t.Errorf("error should name the offending run: %v", err)
	}
}

// TestPerCollectorCorruptPayloadFailsExport: a successful in-scope run
// whose payload cannot be decoded must FAIL the export, not emit a
// zero-row stub.
// Traces: SIGNALS-R080 (issue #322)
func TestPerCollectorCorruptPayloadFailsExport(t *testing.T) {
	store := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	targetID, err := store.UpsertTarget("t", "h", 5432, "d", "u", "disable", "NONE", "", true)
	if err != nil {
		t.Fatalf("UpsertTarget: %v", err)
	}
	if err := store.InsertSnapshot(db.Snapshot{
		ID: "snap-x", TargetID: targetID, CollectedAt: now,
		PGVersion: "16", Payload: []byte(`{}`), SizeBytes: 2,
	}); err != nil {
		t.Fatalf("InsertSnapshot: %v", err)
	}
	run := db.QueryRun{
		ID: "run-corrupt", TargetID: targetID, SnapshotID: "snap-x",
		QueryID: "demo_v1", CollectedAt: now, PGVersion: "16",
		RowCount: 1, Status: "success", CreatedAt: now,
	}
	// Corrupt payload: claims uncompressed but is not valid NDJSON JSON.
	corrupt := db.QueryResult{
		RunID: "run-corrupt", Payload: []byte("\xff\xfe not json"),
		Compressed: false, SizeBytes: 10,
	}
	if err := store.InsertQueryRunBatch([]db.QueryRun{run}, []db.QueryResult{corrupt}); err != nil {
		t.Fatalf("InsertQueryRunBatch: %v", err)
	}

	b := export.NewBuilder(store, "test-instance-id")
	b.SetExportPerCollectorFiles(true)
	var buf bytes.Buffer
	err = b.WriteTo(&buf, export.Options{All: true})
	if err == nil {
		t.Fatal("expected export to FAIL on corrupt payload for successful in-scope run, got nil")
	}
	if !strings.Contains(err.Error(), "run-corrupt") {
		t.Errorf("error should name the offending run: %v", err)
	}
}
