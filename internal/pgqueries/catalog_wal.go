package pgqueries

import "time"

// WAL-archiving and backup-health signals (#304). Read-only recovery-posture
// evidence: archiver progress/failures (pg_stat_archiver) plus the
// archive/recovery settings that determine whether continuous archiving and
// PITR are actually possible. Downstream analysis reasons about recovery
// readiness (archive_mode off, failing archiver, wal_level too low, etc.).
//
// Credential safety: archive_command / restore_command can embed secrets
// (e.g. a cloud credential in the shell command), so this collector emits
// only their PRESENCE as a boolean — never the command string itself.

func init() {
	Register(QueryDef{
		ID:       "wal_archiving_v1",
		Category: "server",
		SQL: `SELECT
			s.archived_count,
			s.failed_count,
			s.last_archived_wal,
			s.last_archived_time,
			s.last_failed_wal,
			s.last_failed_time,
			s.stats_reset,
			current_setting('archive_mode')                                AS archive_mode,
			current_setting('archive_timeout')                             AS archive_timeout,
			current_setting('wal_level')                                   AS wal_level,
			(COALESCE(current_setting('archive_command', true), '') <> '') AS archive_command_configured,
			(COALESCE(current_setting('restore_command', true), '') <> '') AS restore_command_configured
		FROM pg_stat_archiver s`,
		ResultKind:     ResultScalar,
		RetentionClass: RetentionMedium,
		Timeout:        5 * time.Second,
		Cadence:        Cadence15m,
	})
}
