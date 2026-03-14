package pgqueries

import "time"

// Server Survival Pack — diagnostics focused on conditions that can
// severely degrade or bring down a PostgreSQL server if ignored.

func init() {
	// Replication slot risk: stale/inactive slots retain WAL and can
	// fill disk. Returns empty rowset when no slots are configured.
	Register(QueryDef{
		ID:       "replication_slots_risk_v1",
		Category: "replication",
		SQL: `SELECT
			slot_name,
			slot_type,
			active,
			database,
			plugin,
			pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn) AS retained_wal_bytes,
			pg_wal_lsn_diff(pg_current_wal_lsn(), confirmed_flush_lsn) AS unconfirmed_wal_bytes
		FROM pg_replication_slots
		ORDER BY pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn) DESC NULLS LAST`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionShort,
		Timeout:        5 * time.Second,
		Cadence:        Cadence5m,
	})

	// Replication status: connected replicas, lag indicators, sync
	// state. Returns empty rowset on standalone instances.
	Register(QueryDef{
		ID:       "replication_status_v1",
		Category: "replication",
		SQL: `SELECT
			pid,
			usename,
			application_name,
			client_addr,
			state,
			sync_state,
			sent_lsn,
			write_lsn,
			flush_lsn,
			replay_lsn,
			pg_wal_lsn_diff(sent_lsn, replay_lsn) AS replay_lag_bytes,
			pg_wal_lsn_diff(sent_lsn, write_lsn)  AS write_lag_bytes,
			pg_wal_lsn_diff(write_lsn, flush_lsn) AS flush_lag_bytes
		FROM pg_stat_replication
		ORDER BY replay_lag_bytes DESC NULLS LAST`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionShort,
		Timeout:        5 * time.Second,
		Cadence:        Cadence5m,
	})

	// Checkpointer stats: PG 17 split checkpoint columns out of
	// pg_stat_bgwriter into pg_stat_checkpointer. Uses SELECT * for
	// forward compatibility.
	Register(QueryDef{
		ID:           "checkpointer_stats_v1",
		Category:     "server",
		MinPGVersion: 17,
		SQL:          `SELECT * FROM pg_stat_checkpointer`,
		ResultKind:   ResultScalar,
		RetentionClass: RetentionMedium,
		Timeout:        5 * time.Second,
		Cadence:        Cadence15m,
	})

	// Vacuum health: high-signal synthesis over pg_stat_user_tables
	// focused on dead tuple pressure, overdue vacuum/analyze, and
	// autovacuum-disabled tables. Adds XID freeze age context.
	Register(QueryDef{
		ID:       "vacuum_health_v1",
		Category: "tables",
		SQL: `SELECT
			s.schemaname,
			s.relname                                       AS table_name,
			s.n_live_tup,
			s.n_dead_tup,
			CASE WHEN s.n_live_tup + s.n_dead_tup > 0
				THEN round((s.n_dead_tup::numeric
					/ (s.n_live_tup + s.n_dead_tup) * 100)::numeric, 2)
				ELSE 0
			END                                             AS dead_pct,
			s.last_vacuum,
			s.last_autovacuum,
			s.last_analyze,
			s.last_autoanalyze,
			s.vacuum_count,
			s.autovacuum_count,
			age(c.relfrozenxid)                             AS xid_age,
			array_to_string(c.reloptions, ', ')             AS reloptions
		FROM pg_stat_user_tables s
		JOIN pg_class c ON c.oid = s.relid
		WHERE s.n_dead_tup > 0
		   OR s.last_autovacuum IS NULL
		   OR age(c.relfrozenxid) > 500000000
		ORDER BY s.n_dead_tup DESC
		LIMIT 50`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        10 * time.Second,
		Cadence:        Cadence15m,
	})

	// Idle-in-transaction offenders: backends holding open transactions
	// without actively executing queries. These hold locks, block
	// vacuum, and waste connections.
	Register(QueryDef{
		ID:       "idle_in_txn_offenders_v1",
		Category: "activity",
		SQL: `SELECT
			pid,
			usename,
			application_name,
			client_addr,
			state,
			EXTRACT(EPOCH FROM (now() - xact_start))   AS txn_age_seconds,
			EXTRACT(EPOCH FROM (now() - state_change))  AS state_age_seconds,
			LEFT(query, 200)                            AS query_snippet
		FROM pg_stat_activity
		WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')
		  AND pid != pg_backend_pid()
		ORDER BY xact_start ASC NULLS LAST`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionShort,
		Timeout:        5 * time.Second,
		Cadence:        Cadence5m,
	})

	// Database sizes: all non-template databases with sizes for
	// growth monitoring and disk-risk triage.
	Register(QueryDef{
		ID:       "database_sizes_v1",
		Category: "server",
		SQL: `SELECT
			datname                                     AS database_name,
			pg_database_size(datname)                   AS size_bytes,
			datconnlimit                                AS connection_limit,
			age(datfrozenxid)                           AS xid_age
		FROM pg_database
		WHERE datistemplate = false
		ORDER BY pg_database_size(datname) DESC`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        10 * time.Second,
		Cadence:        Cadence1h,
	})

	// Largest relations: top 30 user tables/indexes by total disk
	// size for storage triage.
	Register(QueryDef{
		ID:       "largest_relations_v1",
		Category: "tables",
		SQL: `SELECT
			schemaname,
			relname                                     AS table_name,
			pg_total_relation_size(relid)               AS total_size_bytes,
			pg_relation_size(relid)                     AS table_size_bytes,
			pg_indexes_size(relid)                      AS indexes_size_bytes,
			n_live_tup,
			n_dead_tup
		FROM pg_stat_user_tables
		ORDER BY pg_total_relation_size(relid) DESC
		LIMIT 30`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        10 * time.Second,
		Cadence:        Cadence1h,
	})

	// Temp I/O pressure: per-database temp file usage from
	// pg_stat_database. High temp usage indicates work_mem exhaustion.
	Register(QueryDef{
		ID:       "temp_io_pressure_v1",
		Category: "server",
		SQL: `SELECT
			datname                                     AS database_name,
			temp_files,
			temp_bytes,
			stats_reset
		FROM pg_stat_database
		WHERE datname IS NOT NULL
		  AND (temp_files > 0 OR temp_bytes > 0)
		ORDER BY temp_bytes DESC NULLS LAST`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        5 * time.Second,
		Cadence:        Cadence15m,
	})
}
