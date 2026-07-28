//go:build integration

// Collector output-contract verification against a live PostgreSQL —
// the missing STDD tests->artifact link for issue #314.
//
// The pgqueries unit tests are static (registration, read-only linting,
// ordering); the collector SQL is never run, so the entire internal-
// "char" class defect (#312) was invisible until it surfaced live —
// pg_constraints_v1.contype serialized as the integer 102 instead of
// the string "f", so the Analyzer's `contype == "f"` check skipped
// every FK constraint (Elevarq/Analyzer#1871).
//
// This test closes that gap: it seeds representative schema (a parent +
// child table with an UNINDEXED foreign key) into a real PG, runs the
// full collection, exports a snapshot ZIP via the production
// export.Builder path, reads it back, and asserts the per-collector
// output contract for the catalog/schema collectors:
//
//   - OC-R003 / INV-CHAR-TEXT-VERIFIED: internal-"char" columns
//     (contype, relkind, relpersistence, provolatile, prokind) decode as
//     single-character STRINGS, never JSON numbers; the seeded FK yields
//     a pg_constraints_v1 row with contype == "f" (the #312 lock).
//   - OC-R002 / INV-OUTPUT-CONTRACT: each char-type catalog collector's
//     spec-declared columns are present in its payload objects.
//   - OC-R004 / INV-STATUS-PAYLOAD-VERIFIED: every exported success run
//     with row_count=N has exactly one joinable payload with N rows.
//   - OC-R005 / INV-04 (#319): OID 18 is normalized to text at the
//     connection boundary for ALL collectors — the columns #313's
//     per-column ::text sweep missed (relkind in the bloat collectors,
//     provolatile AS volatility in pg_functions_v1, attidentity in
//     pg_identity_columns_v1) decode as STRINGS, never JSON numbers.
//
// Gated by the `integration` build tag and SIGNALS_TEST_PG_DSN (skips
// locally when unset), matching signals_integration_test.go. In CI it
// runs against a postgres service container matrixed PG 14/15/16/17/18.
//
// Run with:
//
//	SIGNALS_TEST_PG_DSN="postgres://signals@localhost/postgres" \
//	  go test -tags integration ./tests/ \
//	  -run TestIntegration_CollectorOutputContractAgainstRealPG
//
// Spec: specifications/collector-output-contract.md (OC-R001..R005);
//       features/signals/appendix-a-api-contract.md
//       §"Collector output-contract verification (#314)".

package tests

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elevarq/signals/internal/collector"
	"github.com/elevarq/signals/internal/config"
	"github.com/elevarq/signals/internal/export"
)

// internalCharColumns are the PostgreSQL internal-"char" columns the
// #313 fix cast ::text. pgx scans an uncast "char" as an integer byte,
// so any of these appearing as a JSON number in a payload is the #312
// regression. Names are the exported column aliases in the collector
// SQL.
var internalCharColumns = map[string]bool{
	"contype":        true,
	"relkind":        true,
	"relpersistence": true,
	"provolatile":    true,
	"prokind":        true,
}

// declaredColumnsByCollector is the subset of spec-declared columns
// (specifications/collectors/<id>.md "Output columns") asserted present
// for the internal-"char" catalog collectors touched by #313. Kept to
// the stable structural columns so version-gated additions don't make
// the test brittle. OC-R002 / INV-OUTPUT-CONTRACT.
var declaredColumnsByCollector = map[string][]string{
	"pg_constraints_v1": {
		"schemaname", "relname", "conname", "contype", "condef",
		"column_name", "column_position", "relkind",
	},
	"pg_class_storage_v1": {
		"relid", "schemaname", "relname", "relkind", "relpersistence",
		"relispartition", "relhasindex",
	},
}

// exportedRun mirrors the query_runs.ndjson row shape emitted by
// export.Builder.writeQueryRuns.
type exportedRun struct {
	ID       string `json:"id"`
	QueryID  string `json:"query_id"`
	RowCount int    `json:"row_count"`
	Status   string `json:"status"`
	Reason   string `json:"reason"`
	Error    string `json:"error"`
}

// exportedResult mirrors the query_results.ndjson row shape emitted by
// export.Builder.writeQueryResults: run_id + decoded payload objects.
type exportedResult struct {
	RunID   string           `json:"run_id"`
	Payload []map[string]any `json:"payload"`
}

func TestIntegration_CollectorOutputContractAgainstRealPG(t *testing.T) {
	dsn := os.Getenv("SIGNALS_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("SIGNALS_TEST_PG_DSN not set — skipping live PostgreSQL integration test")
	}

	seedRepresentativeSchema(t, dsn)

	connCfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse SIGNALS_TEST_PG_DSN: %v", err)
	}

	// Collection reads the target password from the env var named by
	// PasswordEnv (config.TargetConfig). Prefer an operator-supplied
	// var name; otherwise carry the password from the DSN through a
	// dedicated var so the collector authenticates the same way the
	// seed did. Peer/trust auth leaves both empty and still works.
	passwordEnv := os.Getenv("SIGNALS_TEST_PG_PASSWORD_ENV")
	if passwordEnv == "" && connCfg.Password != "" {
		passwordEnv = "SIGNALS_OC_TEST_PG_PASSWORD"
		t.Setenv(passwordEnv, connCfg.Password)
	}

	// sslmode from the DSN when present (a CI service container is
	// plaintext -> disable); default to prefer so a TLS-capable target
	// still negotiates TLS.
	sslMode := connCfg.RuntimeParams["sslmode"]
	if sslMode == "" {
		if connCfg.TLSConfig == nil {
			sslMode = "disable"
		} else {
			sslMode = "prefer"
		}
	}

	tgt := config.TargetConfig{
		Name:        "output-contract-target",
		Host:        connCfg.Host,
		Port:        int(connCfg.Port),
		DBName:      connCfg.Database,
		User:        connCfg.User,
		SSLMode:     sslMode,
		PasswordEnv: passwordEnv,
		Enabled:     true,
	}

	store := openTestDB(t)

	// Run's immediate baseline cycle executes every eligible collector
	// against a fresh store (no prior run has consumed the daily
	// cadence), so the catalog/schema collectors we assert on all fire
	// on the first cycle.
	c := collector.New(
		store,
		[]config.TargetConfig{tgt},
		24*time.Hour,
		30,
		collector.WithTargetTimeout(90*time.Second),
		collector.WithQueryTimeout(20*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx)
	}()

	// Run kicks off an immediate baseline cycle. Wait for it to land a
	// snapshot rather than sleeping a fixed duration.
	if !waitForSnapshotCount(t, store, 1, 120*time.Second) {
		cancel()
		<-done
		t.Fatalf("initial collection did not produce a snapshot within 120s (connectivity / role-safety failure?)")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("collector goroutine did not stop within 15s of context cancel")
	}

	// Export a snapshot ZIP via the production path and read it back.
	builder := export.NewBuilder(store, "output-contract-test")
	var buf bytes.Buffer
	if err := builder.WriteTo(&buf, export.Options{}); err != nil {
		t.Fatalf("export WriteTo: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	runs := readRuns(t, zr)
	results := readResults(t, zr)

	if len(runs) == 0 {
		t.Fatal("exported snapshot carries no query_runs — collection produced nothing")
	}

	// --- OC-R004 / INV-STATUS-PAYLOAD-VERIFIED ---------------------
	// Every success run with row_count=N joins to exactly one payload
	// with exactly N objects. This is the end-to-end proof of
	// INV-SNAP-STATUS-PAYLOAD (#312) through the real export.
	resultByRunID := map[string]exportedResult{}
	for _, r := range results {
		if _, dup := resultByRunID[r.RunID]; dup {
			t.Errorf("OC-R004: duplicate query_results payload for run_id %s", r.RunID)
		}
		resultByRunID[r.RunID] = r
	}
	for _, run := range runs {
		if run.Status != "success" {
			continue
		}
		res, ok := resultByRunID[run.ID]
		if !ok {
			t.Errorf("OC-R004 (INV-STATUS-PAYLOAD): success run %s (query_id=%s, row_count=%d) has NO joinable query_results payload",
				run.ID, run.QueryID, run.RowCount)
			continue
		}
		if len(res.Payload) != run.RowCount {
			t.Errorf("OC-R004 (INV-STATUS-PAYLOAD): run %s (query_id=%s) declares row_count=%d but payload has %d rows",
				run.ID, run.QueryID, run.RowCount, len(res.Payload))
		}
	}

	// Index successful catalog/schema collector payloads by query_id.
	payloadByQueryID := map[string][]map[string]any{}
	for _, run := range runs {
		if run.Status != "success" {
			continue
		}
		if res, ok := resultByRunID[run.ID]; ok {
			payloadByQueryID[run.QueryID] = res.Payload
		}
	}

	// --- OC-R003 / INV-CHAR-TEXT-VERIFIED --------------------------
	// Across ALL collector payloads, every internal-"char" column that
	// appears must decode as a single-character STRING, never a number.
	charSeen := 0
	for queryID, payload := range payloadByQueryID {
		for i, row := range payload {
			for col, val := range row {
				if !internalCharColumns[col] {
					continue
				}
				if val == nil {
					continue // NULLs are legitimate
				}
				charSeen++
				s, isStr := val.(string)
				if !isStr {
					t.Errorf("OC-R003 (INV-CHAR-TEXT): %s row %d column %q is a %T (%v), want single-char string — the #312 regression (pgx scans \"char\" as int)",
						queryID, i, col, val, val)
					continue
				}
				if len(s) != 1 {
					t.Errorf("OC-R003 (INV-CHAR-TEXT): %s row %d column %q = %q, want a single character",
						queryID, i, col, s)
				}
			}
		}
	}
	if charSeen == 0 {
		t.Error("OC-R003: no internal-\"char\" columns observed in any payload — the seeded schema should have produced pg_constraints_v1/pg_class_storage_v1 rows; harness is not exercising the #312 class")
	}

	// --- OC-R003 (the exact #312 lock): the seeded unindexed FK MUST
	// produce a pg_constraints_v1 row with contype == "f" (string). ---
	constraints, ok := payloadByQueryID["pg_constraints_v1"]
	if !ok {
		t.Fatal("OC-R003: pg_constraints_v1 produced no successful payload — cannot verify the #312 contype==\"f\" lock")
	}
	foundFK := false
	for _, row := range constraints {
		if ct, _ := row["contype"].(string); ct == "f" {
			foundFK = true
			break
		}
	}
	if !foundFK {
		t.Errorf("OC-R003 (#312 LOCK): no pg_constraints_v1 row with contype==\"f\" for the seeded unindexed FK; contype values seen=%v — this is exactly the #312 regression (uncast contype serialized as 102)",
			distinctContype(constraints))
	}

	// --- OC-R005 / INV-04 (the #319 central-fix lock) --------------
	// The columns #313's per-column ::text sweep left uncast must ALSO
	// decode as strings, proving OID 18 is normalized at the connection
	// boundary for ALL collectors, not just the hand-cast ones:
	//
	//   - relkind in the bloat collectors (single-char, e.g. "r"/"i"),
	//   - provolatile AS volatility in pg_functions_v1 (single-char,
	//     e.g. "i"/"s"/"v"),
	//   - attidentity in pg_identity_columns_v1 (a string; "" for the
	//     non-set case, "a"/"d" when set — never a number).
	//
	// These assertions go RED with the OID-18 AfterConnect registration
	// removed (volatility/attidentity would decode as numbers) and GREEN
	// with it in place.
	assertStringCol := func(queryID, col string, allowEmpty bool) {
		payload, ok := payloadByQueryID[queryID]
		if !ok {
			t.Errorf("OC-R005 (#319): expected collector %s to produce a payload against the seeded schema, but none present — cannot verify %q normalizes to a string", queryID, col)
			return
		}
		seen := 0
		for i, row := range payload {
			val, present := row[col]
			if !present || val == nil {
				continue
			}
			seen++
			s, isStr := val.(string)
			if !isStr {
				t.Errorf("OC-R005 (#319 BOUNDARY LOCK): %s row %d column %q is a %T (%v), want a string — the internal \"char\" (OID 18) is NOT normalized at the connection boundary (this column was missed by #313's per-column ::text sweep)",
					queryID, i, col, val, val)
				continue
			}
			if len(s) > 1 {
				t.Errorf("OC-R005 (#319): %s row %d column %q = %q, want a single character (or empty)", queryID, i, col, s)
			}
			if len(s) == 0 && !allowEmpty {
				t.Errorf("OC-R005 (#319): %s row %d column %q is empty, want a single character", queryID, i, col)
			}
		}
		if seen == 0 {
			t.Errorf("OC-R005 (#319): collector %s emitted no %q values — the seeded schema should have exercised it; harness is not covering the columns #313 missed", queryID, col)
		}
	}
	// relkind in the bloat collectors: single-char, non-empty.
	assertStringCol("bloat_estimate_v1", "relkind", false)
	assertStringCol("index_bloat_estimate_v1", "relkind", false)
	// provolatile AS volatility: single-char, non-empty.
	assertStringCol("pg_functions_v1", "volatility", false)
	// attidentity: may be "" (non-set) or a single char (a/d).
	assertStringCol("pg_identity_columns_v1", "attidentity", true)

	// --- OC-R002 / INV-OUTPUT-CONTRACT -----------------------------
	// Each char-type catalog collector's declared columns are present.
	for collectorID, cols := range declaredColumnsByCollector {
		payload, ok := payloadByQueryID[collectorID]
		if !ok {
			t.Errorf("OC-R002: expected collector %s to produce a payload against the seeded schema, but none present", collectorID)
			continue
		}
		if len(payload) == 0 {
			t.Errorf("OC-R002: collector %s produced an empty payload; cannot verify declared columns", collectorID)
			continue
		}
		row := payload[0]
		for _, col := range cols {
			if _, present := row[col]; !present {
				t.Errorf("OC-R002 (INV-OUTPUT-CONTRACT): collector %s payload is missing declared column %q; keys present=%v",
					collectorID, col, keysOf(row))
			}
		}
	}
}

// seedRepresentativeSchema creates a parent table (PK) and a child
// table whose FK to the parent is UNINDEXED, plus a non-PK index and
// columns of varied types. The unindexed FK is what makes
// pg_constraint emit a contype='f' row — the exact #312 fixture.
func seedRepresentativeSchema(t *testing.T, dsn string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("seed: connect: %v", err)
	}
	defer pool.Close()

	// Idempotent seed in a dedicated schema so repeated runs are clean
	// and we never touch the target's own objects.
	stmts := []string{
		`DROP SCHEMA IF EXISTS signals_oc_test CASCADE`,
		`CREATE SCHEMA signals_oc_test`,
		`CREATE TABLE signals_oc_test.parent (
			id          bigint PRIMARY KEY,
			label       text NOT NULL,
			created_at  timestamptz NOT NULL DEFAULT now(),
			ratio       double precision,
			flags       boolean DEFAULT false
		)`,
		// Child table: fk_parent_id references parent(id) with NO index
		// on the FK column -> pg_constraint emits a contype='f' row and
		// the analyzer's missing-FK-index rule has a target.
		`CREATE TABLE signals_oc_test.child (
			id          bigint PRIMARY KEY,
			fk_parent_id bigint NOT NULL
				REFERENCES signals_oc_test.parent(id),
			note        varchar(64),
			amount      numeric(12,2)
		)`,
		// A non-PK index on a NON-FK column, so at least one ordinary
		// index exists without accidentally indexing the FK. This heap
		// table + its index also feed the bloat collectors so their
		// relkind columns ('r'/'i') are exercised (OC-R005 / #319).
		`CREATE INDEX child_note_idx ON signals_oc_test.child (note)`,
		// An identity column so pg_identity_columns_v1 emits attidentity
		// (the raw "char" column #313's per-column sweep missed). The
		// generated column yields attidentity='a'; the plain int/uuid
		// columns above yield the non-set attidentity='' (empty string
		// under text-protocol semantics) — both are exercised. #319.
		`CREATE TABLE signals_oc_test.identity_demo (
			id    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			key   uuid NOT NULL,
			label text
		)`,
		// A SQL function so pg_functions_v1 emits provolatile AS
		// volatility (also raw "char", missed by #313). #319.
		`CREATE FUNCTION signals_oc_test.demo_immutable(x integer)
			RETURNS integer LANGUAGE sql IMMUTABLE
			AS 'SELECT x + 1'`,
		`INSERT INTO signals_oc_test.parent (id, label, ratio)
			VALUES (1, 'a', 1.5), (2, 'b', 2.5)`,
		`INSERT INTO signals_oc_test.child (id, fk_parent_id, note, amount)
			VALUES (10, 1, 'x', 9.99), (11, 2, 'y', 1.00)`,
		`INSERT INTO signals_oc_test.identity_demo (key, label)
			VALUES (gen_random_uuid(), 'r1'), (gen_random_uuid(), 'r2')`,
		// ANALYZE so pg_stat_user_tables / n_live_tup are populated.
		`ANALYZE signals_oc_test.parent`,
		`ANALYZE signals_oc_test.child`,
		`ANALYZE signals_oc_test.identity_demo`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("seed stmt failed: %v\nSQL: %s", err, s)
		}
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer ccancel()
		p, err := pgxpool.New(cctx, dsn)
		if err != nil {
			return
		}
		defer p.Close()
		_, _ = p.Exec(cctx, `DROP SCHEMA IF EXISTS signals_oc_test CASCADE`)
	})
}

func readRuns(t *testing.T, zr *zip.Reader) []exportedRun {
	t.Helper()
	data := readZipFile(t, zr, "query_runs.ndjson")
	var out []exportedRun
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var r exportedRun
		if err := json.Unmarshal(line, &r); err != nil {
			t.Fatalf("decode query_runs row: %v\nline: %s", err, line)
		}
		out = append(out, r)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan query_runs.ndjson: %v", err)
	}
	return out
}

func readResults(t *testing.T, zr *zip.Reader) []exportedResult {
	t.Helper()
	data := readZipFile(t, zr, "query_results.ndjson")
	var out []exportedResult
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var r exportedResult
		if err := json.Unmarshal(line, &r); err != nil {
			t.Fatalf("decode query_results row: %v", err)
		}
		out = append(out, r)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan query_results.ndjson: %v", err)
	}
	return out
}

func distinctContype(rows []map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range rows {
		v := fmt.Sprintf("%v (%T)", r["contype"], r["contype"])
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
