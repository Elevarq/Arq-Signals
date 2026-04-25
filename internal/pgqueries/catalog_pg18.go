package pgqueries

// catalog_pg18.go — version-specific overrides for PostgreSQL 18.
//
// PG 18 changed the column shape of two pg_stat_* views in
// backwards-incompatible ways. Each override below emits the same
// canonical column set the consumer expects (R081: stable logical IDs,
// normalized output across versions); only the SQL underneath differs.

func init() {
	// pg_stat_io: PG 18 split `op_bytes` (a single per-row size) into
	// per-direction byte counters: `read_bytes`, `write_bytes`,
	// `extend_bytes`. The op_bytes column was removed.
	//
	// Canonical schema (see catalog_io.go default SQL): the union of
	// both shapes — emit op_bytes (NULL on PG 18) plus the three new
	// byte counters (NULL on PG 16/17). Same column order, same names,
	// only the populated subset differs by major.
	RegisterOverride(18, "pg_stat_io_v1", `SELECT
		backend_type,
		object,
		context,
		reads,
		read_time,
		writes,
		write_time,
		writebacks,
		writeback_time,
		extends,
		extend_time,
		NULL::bigint AS op_bytes,
		read_bytes,
		write_bytes,
		extend_bytes,
		hits,
		evictions,
		reuses,
		fsyncs,
		fsync_time,
		stats_reset
	FROM pg_stat_io
	ORDER BY backend_type, object, context`)

	// pg_stat_wal: PG 18 renamed wal_write -> wal_writes and
	// wal_sync -> wal_syncs (the timing columns wal_write_time /
	// wal_sync_time were retained). The override aliases the new
	// names back to the canonical schema so downstream consumers see
	// the original column names regardless of server major.
	RegisterOverride(18, "pg_stat_wal_v1", `SELECT
		wal_records,
		wal_fpi,
		wal_bytes,
		wal_buffers_full,
		wal_writes AS wal_write,
		wal_syncs AS wal_sync,
		wal_write_time,
		wal_sync_time,
		stats_reset
	FROM pg_stat_wal`)
}
