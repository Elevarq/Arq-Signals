package pgqueries

import "time"

// Diagnostic Pack 1 — high-value diagnostics beyond the baseline
// stat views. These collectors provide operational, security, and
// planner-health signals.

func init() {
	// Server identity: version, uptime, and database context in a
	// single query. Complements pg_version_v1 which captures only
	// version().
	Register(QueryDef{
		ID:       "server_identity_v1",
		Category: "server",
		SQL: `SELECT
			version()                                              AS full_version,
			current_setting('server_version')                      AS version_string,
			current_setting('server_version_num')::int             AS version_num,
			pg_postmaster_start_time()                             AS started_at,
			EXTRACT(EPOCH FROM (now() - pg_postmaster_start_time())) AS uptime_seconds,
			current_database()                                     AS database_name,
			current_user                                           AS connected_as,
			pg_database_size(current_database())                   AS database_size_bytes`,
		ResultKind:     ResultScalar,
		RetentionClass: RetentionLong,
		Timeout:        5 * time.Second,
		Cadence:        Cadence6h,
	})

	// Extension inventory: all installed extensions with versions.
	Register(QueryDef{
		ID:       "extension_inventory_v1",
		Category: "server",
		SQL: `SELECT
			name,
			default_version,
			installed_version,
			comment
		FROM pg_available_extensions
		WHERE installed_version IS NOT NULL
		ORDER BY name`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionLong,
		Timeout:        5 * time.Second,
		Cadence:        Cadence6h,
	})

	// Checkpoint and background writer health. Uses SELECT * for
	// cross-version compatibility: PG 17 moved checkpoint columns
	// to pg_stat_checkpointer, changing pg_stat_bgwriter's schema.
	Register(QueryDef{
		ID:             "bgwriter_stats_v1",
		Category:       "server",
		SQL:            `SELECT * FROM pg_stat_bgwriter`,
		ResultKind:     ResultScalar,
		RetentionClass: RetentionMedium,
		Timeout:        5 * time.Second,
		Cadence:        Cadence15m,
	})

	// Long-running transactions (older than 5 minutes).
	Register(QueryDef{
		ID:       "long_running_txns_v1",
		Category: "activity",
		SQL: `SELECT
			pid,
			usename,
			application_name,
			client_addr,
			state,
			wait_event_type,
			wait_event,
			EXTRACT(EPOCH FROM (now() - xact_start)) AS txn_age_seconds,
			LEFT(query, 200)                          AS query_snippet
		FROM pg_stat_activity
		WHERE xact_start IS NOT NULL
		  AND state != 'idle'
		  AND now() - xact_start > interval '5 minutes'
		ORDER BY xact_start ASC`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionShort,
		Timeout:        10 * time.Second,
		Cadence:        Cadence5m,
	})

	// Locks and blocking chains. Uses pg_blocking_pids() which is
	// available since PostgreSQL 9.6.
	Register(QueryDef{
		ID:       "blocking_locks_v1",
		Category: "activity",
		SQL: `SELECT
			blocked.pid                             AS blocked_pid,
			blocked.usename                         AS blocked_user,
			LEFT(blocked.query, 200)                AS blocked_query,
			blocking.pid                            AS blocking_pid,
			blocking.usename                        AS blocking_user,
			LEFT(blocking.query, 200)               AS blocking_query,
			blocked.wait_event_type,
			blocked.wait_event,
			EXTRACT(EPOCH FROM (now() - blocked.query_start)) AS waiting_seconds
		FROM pg_stat_activity AS blocked
		JOIN pg_stat_activity AS blocking
			ON blocking.pid = ANY(pg_blocking_pids(blocked.pid))
		WHERE cardinality(pg_blocking_pids(blocked.pid)) > 0
		ORDER BY waiting_seconds DESC`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionShort,
		Timeout:        10 * time.Second,
		Cadence:        Cadence5m,
	})

	// Login roles with dangerous privileges.
	Register(QueryDef{
		ID:       "login_roles_v1",
		Category: "security",
		SQL: `SELECT
			rolname,
			rolsuper,
			rolcreatedb,
			rolcreaterole,
			rolreplication,
			rolbypassrls,
			rolconnlimit,
			rolvaliduntil
		FROM pg_roles
		WHERE rolcanlogin = true
		ORDER BY rolsuper DESC, rolname`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionLong,
		Timeout:        5 * time.Second,
		Cadence:        Cadence6h,
	})

	// Connection utilization: aggregate counts from pg_stat_activity.
	Register(QueryDef{
		ID:       "connection_utilization_v1",
		Category: "activity",
		SQL: `SELECT
			count(*)                                                    AS total_connections,
			count(*) FILTER (WHERE state = 'active')                    AS active,
			count(*) FILTER (WHERE state = 'idle')                      AS idle,
			count(*) FILTER (WHERE state = 'idle in transaction')       AS idle_in_txn,
			count(*) FILTER (WHERE state = 'idle in transaction (aborted)') AS idle_in_txn_aborted,
			(SELECT setting::int FROM pg_settings WHERE name = 'max_connections') AS max_connections,
			round(count(*)::numeric /
				(SELECT setting::int FROM pg_settings WHERE name = 'max_connections') * 100, 2) AS pct_used
		FROM pg_stat_activity
		WHERE backend_type = 'client backend'`,
		ResultKind:     ResultScalar,
		RetentionClass: RetentionShort,
		Timeout:        5 * time.Second,
		Cadence:        Cadence5m,
	})

	// Planner statistics staleness: tables where the planner's row
	// estimates may have drifted significantly from reality.
	Register(QueryDef{
		ID:       "planner_stats_staleness_v1",
		Category: "tables",
		SQL: `SELECT
			s.schemaname,
			s.relname                                               AS table_name,
			c.reltuples::bigint                                     AS estimated_rows,
			c.relpages                                              AS estimated_pages,
			s.n_live_tup                                            AS actual_live_rows,
			s.n_mod_since_analyze                                   AS modifications_since_analyze,
			s.last_analyze,
			s.last_autoanalyze,
			round(
				(ABS(c.reltuples - s.n_live_tup)
				/ NULLIF(GREATEST(c.reltuples, s.n_live_tup), 0) * 100)::numeric
			, 2)                                                    AS estimate_drift_pct
		FROM pg_stat_user_tables s
		JOIN pg_class c ON c.oid = s.relid
		ORDER BY s.n_mod_since_analyze DESC NULLS LAST`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        10 * time.Second,
		Cadence:        Cadence1h,
	})

	// pg_stat_statements reset check. Requires pg_stat_statements
	// extension. pg_stat_statements_info is available in PG 14+.
	Register(QueryDef{
		ID:                "pgss_reset_check_v1",
		Category:          "extensions",
		RequiresExtension: "pg_stat_statements",
		MinPGVersion:      14,
		SQL: `SELECT
			stats_reset,
			EXTRACT(EPOCH FROM (now() - stats_reset)) AS seconds_since_reset
		FROM pg_stat_statements_info`,
		ResultKind:     ResultScalar,
		RetentionClass: RetentionMedium,
		Timeout:        5 * time.Second,
		Cadence:        Cadence1h,
	})
}
