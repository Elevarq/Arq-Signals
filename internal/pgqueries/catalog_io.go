package pgqueries

import "time"

// I/O Calibration Pack — runtime I/O counters that feed the analyzer's
// io-cost-calibration detector. Collectors in this file emit raw cumulative
// counters; deltas are computed analyzer-side per delta-semantics.md.

func init() {
	// pg_stat_io_v1: per (backend_type, object, context) physical I/O
	// counters from pg_stat_io. Introduced in PG 16.
	//
	// op_bytes is emitted so the analyzer can convert block counts to
	// bytes without assuming the default BLCKSZ.
	//
	// Specification: specifications/collectors/pg_stat_io_v1.md
	Register(QueryDef{
		ID:           "pg_stat_io_v1",
		Category:     "io",
		MinPGVersion: 16,
		SQL: `SELECT
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
			op_bytes,
			hits,
			evictions,
			reuses,
			fsyncs,
			fsync_time,
			stats_reset
		FROM pg_stat_io
		ORDER BY backend_type, object, context`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        10 * time.Second,
		Cadence:        Cadence15m,
	})

	// pg_stat_wal_v1: cluster-wide WAL generation, write, and sync counters.
	// Introduced in PG 14. Feeds wal-retention-risk, checkpoint-pressure,
	// and the io-cost-calibration detector's WAL pressure dimension.
	//
	// Specification: specifications/collectors/pg_stat_wal_v1.md
	Register(QueryDef{
		ID:           "pg_stat_wal_v1",
		Category:     "server",
		MinPGVersion: 14,
		SQL: `SELECT
			wal_records,
			wal_fpi,
			wal_bytes,
			wal_buffers_full,
			wal_write,
			wal_sync,
			wal_write_time,
			wal_sync_time,
			stats_reset
		FROM pg_stat_wal`,
		ResultKind:     ResultScalar,
		RetentionClass: RetentionMedium,
		Timeout:        5 * time.Second,
		Cadence:        Cadence15m,
	})
}
