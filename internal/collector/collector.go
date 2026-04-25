package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/elevarq/arq-signals/internal/config"
	"github.com/elevarq/arq-signals/internal/db"
	"github.com/elevarq/arq-signals/internal/pgqueries"
)

// connConfigFunc is the function used to build pgx configs. Overridable for testing.
var connConfigFunc = BuildConnConfig

// Collector handles scheduled PostgreSQL telemetry collection.
type Collector struct {
	db                   *db.DB
	targets              []config.TargetConfig
	interval             time.Duration
	retentionDays        int
	maxConcurrentTargets int
	targetTimeout        time.Duration
	queryTimeout         time.Duration
	allowUnsafeRole        bool
	highSensitivityEnabled bool
	bypassedChecks         []string
	bypassedChecksMu     sync.Mutex
	pools                map[string]*pgxpool.Pool
	poolsMu              sync.Mutex
	collectNowCh         chan struct{}
	entropy              io.Reader
	running              sync.Mutex
}

// lockedRandReader serializes access to a math/rand.Rand source. The
// standard library's *rand.Rand is not safe for concurrent use, but ULID
// generation runs in parallel per target and per query. Without this wrapper
// concurrent ulid.MustNew calls race on the underlying state, occasionally
// producing duplicate IDs.
type lockedRandReader struct {
	mu sync.Mutex
	r  *rand.Rand
}

func (l *lockedRandReader) Read(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.r.Read(p)
}

// New creates a new Collector.
func New(store *db.DB, targets []config.TargetConfig, interval time.Duration, retentionDays int, opts ...CollectorOption) *Collector {
	c := &Collector{
		db:                   store,
		targets:              targets,
		interval:             interval,
		retentionDays:        retentionDays,
		maxConcurrentTargets: 4,
		targetTimeout:        60 * time.Second,
		queryTimeout:         10 * time.Second,
		pools:                make(map[string]*pgxpool.Pool),
		collectNowCh:         make(chan struct{}, 1),
		entropy:              &lockedRandReader{r: rand.New(rand.NewSource(time.Now().UnixNano()))},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// CollectorOption configures a Collector.
type CollectorOption func(*Collector)

// WithMaxConcurrentTargets sets the max number of targets collected in parallel.
func WithMaxConcurrentTargets(n int) CollectorOption {
	return func(c *Collector) {
		if n > 0 {
			c.maxConcurrentTargets = n
		}
	}
}

// WithTargetTimeout sets the per-target collection timeout.
func WithTargetTimeout(d time.Duration) CollectorOption {
	return func(c *Collector) {
		if d > 0 {
			c.targetTimeout = d
		}
	}
}

// WithQueryTimeout sets the per-query timeout.
func WithQueryTimeout(d time.Duration) CollectorOption {
	return func(c *Collector) {
		if d > 0 {
			c.queryTimeout = d
		}
	}
}

// WithAllowUnsafeRole enables collection with unsafe role attributes (lab/dev only).
func WithAllowUnsafeRole(allow bool) CollectorOption {
	return func(c *Collector) {
		c.allowUnsafeRole = allow
	}
}

// GetAllowUnsafeRole returns whether unsafe role mode is enabled.
func (c *Collector) GetAllowUnsafeRole() bool {
	return c.allowUnsafeRole
}

// WithHighSensitivityCollectors enables the four definition-text collectors
// flagged HighSensitivity in the query catalog (R075). Off by default.
func WithHighSensitivityCollectors(enabled bool) CollectorOption {
	return func(c *Collector) {
		c.highSensitivityEnabled = enabled
	}
}

// recordBypassedChecks stores the specific checks that were bypassed in unsafe mode.
func (c *Collector) recordBypassedChecks(checks []string) {
	c.bypassedChecksMu.Lock()
	defer c.bypassedChecksMu.Unlock()
	c.bypassedChecks = append(c.bypassedChecks, checks...)
}

// GetBypassedChecks returns the checks bypassed in unsafe mode.
func (c *Collector) GetBypassedChecks() []string {
	c.bypassedChecksMu.Lock()
	defer c.bypassedChecksMu.Unlock()
	out := make([]string, len(c.bypassedChecks))
	copy(out, c.bypassedChecks)
	return out
}

// Run starts the collection loop, blocking until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) {
	slog.Info("collector starting", "interval", c.interval, "targets", len(c.targets))

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Initial collection — force all queries as a baseline.
	c.runCycle(ctx, true)

	for {
		select {
		case <-ctx.Done():
			slog.Info("collector stopping")
			c.closePools()
			return
		case <-ticker.C:
			c.runCycle(ctx, false)
		case <-c.collectNowCh:
			slog.Info("on-demand collection triggered")
			c.runCycle(ctx, true)
		}
	}
}

// CollectNow triggers an immediate collection cycle (non-blocking).
func (c *Collector) CollectNow() {
	select {
	case c.collectNowCh <- struct{}{}:
	default:
		// Already pending.
	}
}

// LastCollected returns the most recent snapshot time, or empty string.
func (c *Collector) LastCollected() string {
	var ts string
	row := c.db.SQL().QueryRow("SELECT collected_at FROM snapshots ORDER BY collected_at DESC LIMIT 1")
	row.Scan(&ts) // ignore error, empty is fine
	return ts
}

// runCycle runs a collection cycle with overlap protection.
// If forceAll is true, all eligible queries are executed regardless of cadence.
func (c *Collector) runCycle(ctx context.Context, forceAll bool) {
	if !c.running.TryLock() {
		slog.Warn("collection cycle skipped — previous cycle still running")
		return
	}
	defer c.running.Unlock()

	start := time.Now()

	// Build list of enabled targets.
	var enabled []config.TargetConfig
	for _, tgt := range c.targets {
		if tgt.Enabled {
			enabled = append(enabled, tgt)
		}
	}

	// Worker pool: bounded channel semaphore + WaitGroup.
	sem := make(chan struct{}, c.maxConcurrentTargets)
	var wg sync.WaitGroup
	for _, tgt := range enabled {
		tgt := tgt // capture
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			tgtCtx := ctx
			if c.targetTimeout > 0 {
				var cancel context.CancelFunc
				tgtCtx, cancel = context.WithTimeout(ctx, c.targetTimeout)
				defer cancel()
			}

			if err := c.collectTarget(tgtCtx, tgt, forceAll); err != nil {
				slog.Error("collection failed", "target", tgt.Name, "err", err)
				c.db.InsertEvent("collect_error", fmt.Sprintf("target=%s err=%v", tgt.Name, err))
			}
		}()
	}
	wg.Wait()

	c.cleanup()

	slog.Info("collection cycle completed", "duration_ms", time.Since(start).Milliseconds(), "targets", len(enabled))
}

func (c *Collector) collectTarget(ctx context.Context, tgt config.TargetConfig, forceAll bool) error {
	pool, err := c.getPool(ctx, tgt)
	if err != nil {
		return fmt.Errorf("connect %s: %w", tgt.Name, err)
	}

	// Register/update target in DB — store only non-secret metadata.
	targetID, err := c.db.UpsertTarget(
		tgt.Name, tgt.Host, tgt.Port, tgt.DBName, tgt.User,
		tgt.SSLMode, tgt.SecretType(), tgt.SecretRef(), tgt.Enabled,
	)
	if err != nil {
		return fmt.Errorf("upsert target %s: %w", tgt.Name, err)
	}

	// --- Runtime safety validation (fail-closed) ---
	safetyResult, safetyErr := ValidateRoleSafety(ctx, pool)
	if safetyErr != nil {
		return fmt.Errorf("safety validation failed for %s: %w", tgt.Name, safetyErr)
	}

	// Log warnings (non-blocking).
	for _, w := range safetyResult.Warnings {
		slog.Warn("safety hygiene warning", "target", tgt.Name, "warning", w)
	}

	// Check hard failures.
	if !safetyResult.IsSafe() {
		if c.allowUnsafeRole {
			slog.Warn("UNSAFE MODE: bypassing safety checks — not recommended for production",
				"target", tgt.Name, "bypassed_checks", safetyResult.HardFailures)
			// Record bypassed checks for export metadata.
			c.recordBypassedChecks(safetyResult.HardFailures)
		} else {
			return fmt.Errorf("collection blocked for target %s: %s", tgt.Name, safetyResult.Error())
		}
	}

	// Acquire a dedicated connection from the pool. This ensures all
	// safety checks, timeouts, and queries execute on the SAME connection.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection for %s: %w", tgt.Name, err)
	}
	defer conn.Release()

	// Verify session read-only posture on this specific connection.
	var readOnly string
	if err := conn.QueryRow(ctx, "SHOW default_transaction_read_only").Scan(&readOnly); err != nil {
		return fmt.Errorf("session safety check failed for %s: cannot verify read-only posture: %w", tgt.Name, err)
	}
	if readOnly != "on" {
		return fmt.Errorf("session safety check failed for %s: session is not read-only (default_transaction_read_only=%s)", tgt.Name, readOnly)
	}

	// Begin a READ ONLY transaction on this dedicated connection.
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Apply timeouts via SET LOCAL inside this transaction. SET LOCAL
	// ensures timeouts apply to exactly this transaction on this connection
	// and are automatically reset when the transaction ends.
	stmtTimeoutMs := int(c.queryTimeout.Milliseconds())
	lockTimeoutMs := 5000 // 5 seconds — conservative default
	idleTimeoutMs := int(c.targetTimeout.Milliseconds())
	for _, t := range []struct {
		param string
		value int
	}{
		{"statement_timeout", stmtTimeoutMs},
		{"lock_timeout", lockTimeoutMs},
		{"idle_in_transaction_session_timeout", idleTimeoutMs},
	} {
		if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL %s = %d", t.param, t.value)); err != nil {
			slog.Warn("failed to SET LOCAL timeout", "param", t.param, "target", tgt.Name, "err", err)
		}
	}

	// Step 1: Get PG version.
	var versionStr string
	if err := tx.QueryRow(ctx, "SELECT version()").Scan(&versionStr); err != nil {
		return fmt.Errorf("query version: %w", err)
	}
	pgMajor := parsePGMajorVersion(versionStr)

	// Step 2: Detect installed extensions.
	extensions := detectExtensions(ctx, tx)

	// Step 3: Filter eligible queries.
	filterParams := pgqueries.FilterParams{
		PGMajorVersion:         pgMajor,
		Extensions:             extensions,
		HighSensitivityEnabled: c.highSensitivityEnabled,
	}
	eligible := pgqueries.Filter(filterParams)
	gatedHighSensitivityIDs := pgqueries.HighSensitivityIDs(filterParams)

	// Step 3b: Apply cadence planner unless forceAll.
	queries := eligible
	if !forceAll {
		lastRuns, lrErr := c.db.GetLastRunTimes(targetID)
		if lrErr != nil {
			slog.Warn("cadence planner: GetLastRunTimes failed, running all eligible", "target", tgt.Name, "err", lrErr)
		} else {
			queries = pgqueries.SelectDue(time.Now().UTC(), eligible, lastRuns)
			if len(queries) == 0 {
				slog.Debug("no queries due this cycle", "target", tgt.Name, "eligible", len(eligible))
				tx.Rollback(ctx)
				return nil
			}
		}
	}

	now := time.Now().UTC()
	snapID := ulid.MustNew(ulid.Timestamp(now), c.entropy).String()
	collectedAt := now.Format(time.RFC3339)

	data := &SnapshotData{Version: versionStr}
	var runs []db.QueryRun
	var results []db.QueryResult

	// Step 4: Execute each query with budget-aware timeout.
	for _, q := range queries {
		// Check if target context is already expired.
		if ctx.Err() != nil {
			slog.Warn("target budget exhausted, skipping remaining queries",
				"target", tgt.Name, "query", q.ID, "remaining", len(queries))
			break
		}

		// Per-query timeout: min(queryTimeout, q.Timeout, remaining target budget).
		qTimeout := c.queryTimeout
		if q.Timeout > 0 && q.Timeout < qTimeout {
			qTimeout = q.Timeout
		}
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining < qTimeout {
				qTimeout = remaining
			}
		}

		qCtx, qCancel := context.WithTimeout(ctx, qTimeout)
		start := time.Now()

		// Use a savepoint so a single query failure does not abort
		// the entire READ ONLY transaction (PostgreSQL marks the
		// transaction as aborted after any error).
		savepointName := fmt.Sprintf("arq_q_%d", len(runs))
		tx.Exec(ctx, "SAVEPOINT "+savepointName)

		rows, qErr := queryToMaps(qCtx, tx, q.SQL)
		elapsed := time.Since(start)
		qTimedOut := qCtx.Err() == context.DeadlineExceeded
		qCancel()

		if qErr != nil {
			// Roll back to the savepoint to recover the transaction.
			tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+savepointName)
		}
		tx.Exec(ctx, "RELEASE SAVEPOINT "+savepointName)

		runID := ulid.MustNew(ulid.Timestamp(now), c.entropy).String()

		run := db.QueryRun{
			ID:          runID,
			TargetID:    targetID,
			SnapshotID:  snapID,
			QueryID:     q.ID,
			CollectedAt: collectedAt,
			PGVersion:   versionStr,
			DurationMS:  int(elapsed.Milliseconds()),
			CreatedAt:   collectedAt,
		}

		if qErr != nil {
			run.Error = qErr.Error()
			run.Status = "failed"
			run.Reason = classifyRunError(run.Error)
			if isPermissionDenied(qErr) {
				slog.Warn("query permission denied — grant pg_monitor to the monitoring role",
					"query", q.ID, "target", tgt.Name)
			} else if ctx.Err() != nil {
				slog.Warn("query timed out (target budget exhausted)",
					"query", q.ID, "target", tgt.Name, "duration_ms", elapsed.Milliseconds())
			} else if qTimedOut {
				slog.Warn("query timed out",
					"query", q.ID, "target", tgt.Name, "timeout", qTimeout, "duration_ms", elapsed.Milliseconds())
			} else {
				slog.Warn("query failed", "query", q.ID, "target", tgt.Name, "err", qErr)
			}
			runs = append(runs, run)

			// If target context expired, stop processing more queries.
			if ctx.Err() != nil {
				break
			}
			continue
		}

		run.RowCount = len(rows)
		run.Status = "success"
		runs = append(runs, run)

		// Encode result as NDJSON.
		payload, compressed, sizeBytes, encErr := db.EncodeNDJSON(rows)
		if encErr != nil {
			slog.Warn("encode failed", "query", q.ID, "err", encErr)
			continue
		}

		results = append(results, db.QueryResult{
			RunID:      runID,
			Payload:    payload,
			Compressed: compressed,
			SizeBytes:  sizeBytes,
		})

		// Populate legacy SnapshotData for backward compatibility.
		populateSnapshotField(data, q.ID, rows)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx for %s: %w", tgt.Name, err)
	}

	// Step 4b: Record gated high-sensitivity collectors as skipped runs so
	// collector_status.json contains exactly one entry per registered
	// collector that was relevant to this target. The operator can see the
	// gate is active without having to compare against the registry.
	for _, id := range gatedHighSensitivityIDs {
		runID := ulid.MustNew(ulid.Timestamp(now), c.entropy).String()
		runs = append(runs, db.QueryRun{
			ID:          runID,
			TargetID:    targetID,
			SnapshotID:  snapID,
			QueryID:     id,
			CollectedAt: collectedAt,
			PGVersion:   versionStr,
			CreatedAt:   collectedAt,
			Status:      "skipped",
			Reason:      "config_disabled",
		})
	}

	// Step 5: Batch insert query runs + results.
	if len(runs) > 0 {
		if err := c.db.InsertQueryRunBatch(runs, results); err != nil {
			slog.Error("insert query runs failed", "target", tgt.Name, "err", err)
		}
	}

	// Step 6: Still populate monolithic snapshot for backward compatibility.
	payload, err := MarshalPayload(data)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	snap := db.Snapshot{
		ID:          snapID,
		TargetID:    targetID,
		CollectedAt: collectedAt,
		PGVersion:   data.Version,
		Payload:     json.RawMessage(payload),
		SizeBytes:   len(payload),
	}

	if err := c.db.InsertSnapshot(snap); err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}

	slog.Info("snapshot collected", "target", tgt.Name, "id", snap.ID, "size", snap.SizeBytes,
		"pg_version", data.Version, "queries_due", len(runs), "queries_eligible", len(eligible))
	c.db.InsertTargetEvent(targetID, "snapshot_collected", fmt.Sprintf("target=%s id=%s queries=%d", tgt.Name, snap.ID, len(runs)))

	return nil
}

func (c *Collector) getPool(ctx context.Context, tgt config.TargetConfig) (*pgxpool.Pool, error) {
	c.poolsMu.Lock()
	defer c.poolsMu.Unlock()

	if pool, ok := c.pools[tgt.Name]; ok {
		return pool, nil
	}

	// Build connection config from structured fields, resolving secrets at runtime.
	connCfg, err := connConfigFunc(tgt)
	if err != nil {
		return nil, err
	}

	poolCfg, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, err
	}
	poolCfg.ConnConfig = connCfg
	poolCfg.MaxConns = 2

	// Re-resolve password on each new connection to support rotation.
	poolCfg.BeforeConnect = func(ctx context.Context, cfg *pgx.ConnConfig) error {
		password, err := ResolvePassword(tgt)
		if err != nil {
			slog.Error("failed to resolve password for target", "target", tgt.Name, "err", redactError(err))
			return fmt.Errorf("resolve password: %w", redactError(err))
		}
		cfg.Password = password
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	c.pools[tgt.Name] = pool
	return pool, nil
}

func (c *Collector) closePools() {
	c.poolsMu.Lock()
	defer c.poolsMu.Unlock()

	for name, pool := range c.pools {
		pool.Close()
		slog.Info("closed pool", "target", name)
	}
}

func (c *Collector) cleanup() {
	if c.retentionDays <= 0 {
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -c.retentionDays).Format(time.RFC3339)

	deleted, err := c.db.DeleteSnapshotsOlderThan(cutoff)
	if err != nil {
		slog.Error("snapshot cleanup failed", "err", err)
	} else if deleted > 0 {
		slog.Info("snapshot cleanup complete", "deleted", deleted, "cutoff", cutoff)
	}

	deletedRuns, err := c.db.DeleteQueryRunsOlderThan(cutoff)
	if err != nil {
		slog.Error("query runs cleanup failed", "err", err)
	} else if deletedRuns > 0 {
		slog.Info("query runs cleanup complete", "deleted", deletedRuns, "cutoff", cutoff)
	}
}

// isPermissionDenied returns true if the error is a PostgreSQL
// "insufficient_privilege" error (SQLSTATE 42501).
func isPermissionDenied(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42501"
	}
	return false
}

// queryToMaps executes a query and returns each row as a map[string]any.
func queryToMaps(ctx context.Context, tx pgx.Tx, query string) ([]map[string]any, error) {
	rows, err := tx.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	descs := rows.FieldDescriptions()
	var result []map[string]any

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		m := make(map[string]any, len(descs))
		for i, desc := range descs {
			m[desc.Name] = values[i]
		}
		result = append(result, m)
	}

	return result, rows.Err()
}
