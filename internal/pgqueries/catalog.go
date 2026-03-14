package pgqueries

import "time"

func init() {
	Register(QueryDef{
		ID:             "pg_version_v1",
		Category:       "server",
		SQL:            `SELECT version()`,
		ResultKind:     ResultScalar,
		RetentionClass: RetentionLong,
		Timeout:        5 * time.Second,
		Cadence:        Cadence6h,
	})

	Register(QueryDef{
		ID:       "pg_settings_v1",
		Category: "server",
		SQL: `SELECT name, setting, unit, category, source, pending_restart
		FROM pg_settings ORDER BY name`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionLong,
		Timeout:        10 * time.Second,
		Cadence:        Cadence6h,
	})

	Register(QueryDef{
		ID:       "pg_stat_activity_v1",
		Category: "activity",
		SQL: `SELECT pid, datname, usename, application_name, client_addr,
		backend_start, xact_start, query_start, state_change, wait_event_type,
		wait_event, state, backend_type
		FROM pg_stat_activity
		WHERE pid != pg_backend_pid()
		ORDER BY pid`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionShort,
		Timeout:        10 * time.Second,
		Cadence:        Cadence5m,
	})

	Register(QueryDef{
		ID:       "pg_stat_database_v1",
		Category: "database",
		SQL: `SELECT datid, datname, numbackends, xact_commit, xact_rollback,
		blks_read, blks_hit, tup_returned, tup_fetched, tup_inserted, tup_updated,
		tup_deleted, conflicts, temp_files, temp_bytes, deadlocks, blk_read_time,
		blk_write_time, stats_reset
		FROM pg_stat_database
		WHERE datname IS NOT NULL
		ORDER BY datname`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        10 * time.Second,
		Cadence:        Cadence15m,
	})

	Register(QueryDef{
		ID:       "pg_stat_user_tables_v1",
		Category: "tables",
		SQL: `SELECT schemaname, relname, seq_scan, seq_tup_read,
		idx_scan, idx_tup_fetch, n_tup_ins, n_tup_upd, n_tup_del, n_tup_hot_upd,
		n_live_tup, n_dead_tup, last_vacuum, last_autovacuum, last_analyze,
		last_autoanalyze, vacuum_count, autovacuum_count, analyze_count,
		autoanalyze_count
		FROM pg_stat_user_tables
		ORDER BY schemaname, relname`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        10 * time.Second,
	})

	Register(QueryDef{
		ID:       "pg_stat_user_indexes_v1",
		Category: "indexes",
		SQL: `SELECT schemaname, relname, indexrelname, idx_scan,
		idx_tup_read, idx_tup_fetch
		FROM pg_stat_user_indexes
		ORDER BY schemaname, relname, indexrelname`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        10 * time.Second,
	})

	Register(QueryDef{
		ID:       "pg_statio_user_tables_v1",
		Category: "io",
		SQL: `SELECT schemaname, relname, heap_blks_read,
		heap_blks_hit, idx_blks_read, idx_blks_hit, toast_blks_read,
		toast_blks_hit, tidx_blks_read, tidx_blks_hit
		FROM pg_statio_user_tables
		ORDER BY schemaname, relname`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        10 * time.Second,
	})

	Register(QueryDef{
		ID:       "pg_statio_user_indexes_v1",
		Category: "io",
		SQL: `SELECT schemaname, relname, indexrelname,
		idx_blks_read, idx_blks_hit
		FROM pg_statio_user_indexes
		ORDER BY schemaname, relname, indexrelname`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        10 * time.Second,
	})

	// pg_stat_statements uses SELECT * for cross-version compatibility.
	// The view schema varies across PostgreSQL and extension versions
	// (e.g. blk_read_time was renamed to shared_blk_read_time in PG 17).
	// The collector captures whatever columns the installed version
	// exposes and serializes them dynamically using actual column names.
	Register(QueryDef{
		ID:                "pg_stat_statements_v1",
		Category:          "extensions",
		RequiresExtension: "pg_stat_statements",
		SQL: `SELECT *
		FROM pg_stat_statements
		ORDER BY total_exec_time DESC
		LIMIT 100`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        10 * time.Second,
		Cadence:        Cadence15m,
	})
}
