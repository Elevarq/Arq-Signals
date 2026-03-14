package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a sql.DB with Arq Signals-specific operations.
type DB struct {
	sql *sql.DB
}

// Open creates or opens the SQLite database at path.
func Open(path string, wal bool) (*DB, error) {
	dsn := path + "?_busy_timeout=5000&_foreign_keys=on"
	if wal {
		dsn += "&_journal_mode=WAL"
	}

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(1)

	// Enable WAL via pragma as well (some drivers need this).
	if wal {
		if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("enable WAL: %w", err)
		}
	}

	return &DB{sql: sqlDB}, nil
}

// Close closes the underlying database.
func (d *DB) Close() error {
	return d.sql.Close()
}

// SQL returns the underlying *sql.DB for advanced use.
func (d *DB) SQL() *sql.DB {
	return d.sql
}

// Migrate runs all embedded SQL migrations in order.
func (d *DB) Migrate() error {
	// Create migration tracking table.
	if _, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		var applied int
		err := d.sql.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE filename = ?", entry.Name()).Scan(&applied)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", entry.Name(), err)
		}
		if applied > 0 {
			continue
		}

		data, err := fs.ReadFile(migrationsFS, migrationsDir+"/"+entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		slog.Info("applying migration", "file", entry.Name())
		if err := d.applyMigration(entry.Name(), string(data)); err != nil {
			return err
		}
	}

	return nil
}

// applyMigration runs a single migration file inside a transaction.
// Both the DDL statements and the schema_migrations insert are committed atomically.
func (d *DB) applyMigration(filename, sql string) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", filename, err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(sql); err != nil {
		return fmt.Errorf("apply migration %s: %w", filename, err)
	}

	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (filename, applied_at) VALUES (?, ?)",
		filename, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("record migration %s: %w", filename, err)
	}

	return tx.Commit()
}

// ApplyMigrationSQL exposes applyMigration for testing.
func (d *DB) ApplyMigrationSQL(filename, sql string) error {
	return d.applyMigration(filename, sql)
}

// --- Meta CRUD ---

func (d *DB) GetMeta(key string) (string, error) {
	var val string
	err := d.sql.QueryRow("SELECT value FROM meta WHERE key = ?", key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (d *DB) SetMeta(key, value string) error {
	_, err := d.sql.Exec(
		"INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}

// EnsureInstanceID returns the instance ID, generating one if it doesn't exist.
func (d *DB) EnsureInstanceID() (string, error) {
	id, err := d.GetMeta("instance_id")
	if err != nil {
		return "", err
	}
	if id != "" {
		return id, nil
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate instance id: %w", err)
	}
	id = hex.EncodeToString(b)
	if err := d.SetMeta("instance_id", id); err != nil {
		return "", err
	}
	slog.Info("generated instance ID", "id", id)
	return id, nil
}

// --- Events ---

func (d *DB) InsertEvent(eventType, detail string) error {
	_, err := d.sql.Exec(
		"INSERT INTO events (timestamp, event_type, detail) VALUES (?, ?, ?)",
		time.Now().UTC().Format(time.RFC3339), eventType, detail,
	)
	return err
}

// InsertTargetEvent inserts an event scoped to a specific target.
func (d *DB) InsertTargetEvent(targetID int64, eventType, detail string) error {
	_, err := d.sql.Exec(
		"INSERT INTO events (timestamp, event_type, detail, target_id) VALUES (?, ?, ?, ?)",
		time.Now().UTC().Format(time.RFC3339), eventType, detail, targetID,
	)
	return err
}

// --- Targets ---

type Target struct {
	ID         int64
	Name       string
	Host       string
	Port       int
	DBName     string
	Username   string
	SSLMode    string
	SecretType string
	SecretRef  string
	Enabled    bool
	CreatedAt  string
	UpdatedAt  string
}

func (d *DB) UpsertTarget(name string, host string, port int, dbname string, username string, sslmode string, secretType string, secretRef string, enabled bool) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := d.sql.Exec(`
		INSERT INTO targets (name, dsn_hash, host, port, dbname, username, sslmode, secret_type, secret_ref, enabled, created_at, updated_at)
		VALUES (?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			host=excluded.host, port=excluded.port, dbname=excluded.dbname, username=excluded.username,
			sslmode=excluded.sslmode, secret_type=excluded.secret_type, secret_ref=excluded.secret_ref,
			enabled=excluded.enabled, updated_at=excluded.updated_at`,
		name, host, port, dbname, username, sslmode, secretType, secretRef, enabled, now, now,
	)
	if err != nil {
		return 0, err
	}

	// If it was an update, fetch the existing ID.
	id, err := res.LastInsertId()
	if err != nil || id == 0 {
		var tid int64
		if err2 := d.sql.QueryRow("SELECT id FROM targets WHERE name = ?", name).Scan(&tid); err2 != nil {
			return 0, err2
		}
		return tid, nil
	}
	return id, nil
}

func (d *DB) GetTargets() ([]Target, error) {
	rows, err := d.sql.Query(`SELECT id, name, host, port, dbname, username, sslmode, secret_type, secret_ref, enabled, created_at, updated_at
		FROM targets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []Target
	for rows.Next() {
		var t Target
		if err := rows.Scan(&t.ID, &t.Name, &t.Host, &t.Port, &t.DBName, &t.Username, &t.SSLMode, &t.SecretType, &t.SecretRef, &t.Enabled, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// --- Snapshots ---

type Snapshot struct {
	ID          string
	TargetID    int64
	CollectedAt string
	PGVersion   string
	Payload     json.RawMessage
	SizeBytes   int
}

func (d *DB) InsertSnapshot(s Snapshot) error {
	_, err := d.sql.Exec(
		"INSERT INTO snapshots (id, target_id, collected_at, pg_version, payload, size_bytes) VALUES (?, ?, ?, ?, ?, ?)",
		s.ID, s.TargetID, s.CollectedAt, s.PGVersion, string(s.Payload), s.SizeBytes,
	)
	return err
}

func (d *DB) GetSnapshotsByTarget(targetID int64, since, until string) ([]Snapshot, error) {
	query := "SELECT id, target_id, collected_at, pg_version, payload, size_bytes FROM snapshots WHERE target_id = ?"
	args := []any{targetID}
	if since != "" {
		query += " AND collected_at >= ?"
		args = append(args, since)
	}
	if until != "" {
		query += " AND collected_at <= ?"
		args = append(args, until)
	}
	query += " ORDER BY collected_at"

	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snaps []Snapshot
	for rows.Next() {
		var s Snapshot
		var payload string
		if err := rows.Scan(&s.ID, &s.TargetID, &s.CollectedAt, &s.PGVersion, &payload, &s.SizeBytes); err != nil {
			return nil, err
		}
		s.Payload = json.RawMessage(payload)
		snaps = append(snaps, s)
	}
	return snaps, rows.Err()
}

func (d *DB) GetAllSnapshots(since, until string) ([]Snapshot, error) {
	query := "SELECT id, target_id, collected_at, pg_version, payload, size_bytes FROM snapshots WHERE 1=1"
	var args []any
	if since != "" {
		query += " AND collected_at >= ?"
		args = append(args, since)
	}
	if until != "" {
		query += " AND collected_at <= ?"
		args = append(args, until)
	}
	query += " ORDER BY collected_at"

	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snaps []Snapshot
	for rows.Next() {
		var s Snapshot
		var payload string
		if err := rows.Scan(&s.ID, &s.TargetID, &s.CollectedAt, &s.PGVersion, &payload, &s.SizeBytes); err != nil {
			return nil, err
		}
		s.Payload = json.RawMessage(payload)
		snaps = append(snaps, s)
	}
	return snaps, rows.Err()
}

func (d *DB) CountSnapshots() (int, error) {
	var count int
	err := d.sql.QueryRow("SELECT COUNT(*) FROM snapshots").Scan(&count)
	return count, err
}

func (d *DB) DeleteSnapshotsOlderThan(before string) (int64, error) {
	res, err := d.sql.Exec("DELETE FROM snapshots WHERE collected_at < ?", before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- Query Catalog ---

type QueryCatalogRow struct {
	QueryID        string
	Category       string
	ResultKind     string
	RetentionClass string
	RegisteredAt   string
}

func (d *DB) UpsertQueryCatalog(row QueryCatalogRow) error {
	_, err := d.sql.Exec(`
		INSERT INTO query_catalog (query_id, category, result_kind, retention_class, registered_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(query_id) DO UPDATE SET
			category=excluded.category, result_kind=excluded.result_kind,
			retention_class=excluded.retention_class`,
		row.QueryID, row.Category, row.ResultKind, row.RetentionClass, row.RegisteredAt,
	)
	return err
}

func (d *DB) GetQueryCatalog() ([]QueryCatalogRow, error) {
	rows, err := d.sql.Query("SELECT query_id, category, result_kind, retention_class, registered_at FROM query_catalog ORDER BY query_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []QueryCatalogRow
	for rows.Next() {
		var r QueryCatalogRow
		if err := rows.Scan(&r.QueryID, &r.Category, &r.ResultKind, &r.RetentionClass, &r.RegisteredAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- Query Runs + Results ---

type QueryRun struct {
	ID          string
	TargetID    int64
	SnapshotID  string
	QueryID     string
	CollectedAt string
	PGVersion   string
	DurationMS  int
	RowCount    int
	Error       string
	CreatedAt   string
}

type QueryResult struct {
	RunID      string
	Payload    []byte
	Compressed bool
	SizeBytes  int
}

func (d *DB) InsertQueryRunBatch(runs []QueryRun, results []QueryResult) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	runStmt, err := tx.Prepare(`INSERT INTO query_runs
		(id, target_id, snapshot_id, query_id, collected_at, pg_version, duration_ms, row_count, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer runStmt.Close()

	resStmt, err := tx.Prepare(`INSERT INTO query_results (run_id, payload, compressed, size_bytes) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer resStmt.Close()

	for _, r := range runs {
		if _, err := runStmt.Exec(r.ID, r.TargetID, r.SnapshotID, r.QueryID, r.CollectedAt, r.PGVersion, r.DurationMS, r.RowCount, r.Error, r.CreatedAt); err != nil {
			return fmt.Errorf("insert run %s: %w", r.ID, err)
		}
	}

	for _, res := range results {
		comp := 0
		if res.Compressed {
			comp = 1
		}
		if _, err := resStmt.Exec(res.RunID, res.Payload, comp, res.SizeBytes); err != nil {
			return fmt.Errorf("insert result %s: %w", res.RunID, err)
		}
	}

	return tx.Commit()
}

func (d *DB) GetAllQueryRuns(since, until string) ([]QueryRun, error) {
	query := "SELECT id, target_id, snapshot_id, query_id, collected_at, pg_version, duration_ms, row_count, error, created_at FROM query_runs WHERE 1=1"
	var args []any
	if since != "" {
		query += " AND collected_at >= ?"
		args = append(args, since)
	}
	if until != "" {
		query += " AND collected_at <= ?"
		args = append(args, until)
	}
	query += " ORDER BY collected_at"
	return d.scanQueryRuns(query, args...)
}

func (d *DB) scanQueryRuns(query string, args ...any) ([]QueryRun, error) {
	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []QueryRun
	for rows.Next() {
		var r QueryRun
		if err := rows.Scan(&r.ID, &r.TargetID, &r.SnapshotID, &r.QueryID, &r.CollectedAt, &r.PGVersion, &r.DurationMS, &r.RowCount, &r.Error, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) GetQueryResultByRunID(runID string) (*QueryResult, error) {
	row := d.sql.QueryRow("SELECT run_id, payload, compressed, size_bytes FROM query_results WHERE run_id = ?", runID)
	var res QueryResult
	var comp int
	err := row.Scan(&res.RunID, &res.Payload, &comp, &res.SizeBytes)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	res.Compressed = comp == 1
	return &res, nil
}

func (d *DB) DeleteQueryRunsOlderThan(before string) (int64, error) {
	// Delete results first (FK dependency).
	_, err := d.sql.Exec(`DELETE FROM query_results WHERE run_id IN
		(SELECT id FROM query_runs WHERE collected_at < ?)`, before)
	if err != nil {
		return 0, err
	}
	res, err := d.sql.Exec("DELETE FROM query_runs WHERE collected_at < ?", before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetLastRunTimes returns the most recent successful collected_at per query_id for a target.
func (d *DB) GetLastRunTimes(targetID int64) (map[string]time.Time, error) {
	rows, err := d.sql.Query(
		`SELECT query_id, MAX(collected_at) FROM query_runs
		 WHERE target_id = ? AND error = ''
		 GROUP BY query_id`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]time.Time)
	for rows.Next() {
		var qid, ts string
		if err := rows.Scan(&qid, &ts); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		out[qid] = t
	}
	return out, rows.Err()
}

// GetTargetLastCollected returns the most recent collected_at timestamp for the given target,
// checking both snapshots and query_runs tables. Returns empty string if none found.
func (d *DB) GetTargetLastCollected(targetID int64) string {
	var ts string
	// Check snapshots first.
	_ = d.sql.QueryRow(
		"SELECT collected_at FROM snapshots WHERE target_id = ? ORDER BY collected_at DESC LIMIT 1",
		targetID,
	).Scan(&ts)
	// Check query_runs for a potentially newer timestamp.
	var qrTS string
	_ = d.sql.QueryRow(
		"SELECT collected_at FROM query_runs WHERE target_id = ? ORDER BY collected_at DESC LIMIT 1",
		targetID,
	).Scan(&qrTS)
	if qrTS > ts {
		ts = qrTS
	}
	return ts
}
