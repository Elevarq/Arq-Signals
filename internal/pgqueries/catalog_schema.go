package pgqueries

import "time"

// Schema Metadata Collectors — Phase 1
//
// These collectors extract structural schema metadata on a slow
// cadence (default 24h). They focus on non-system objects only.
//
// Specifications:
//   specifications/collectors/pg_constraints_v1.md
//   specifications/collectors/pg_indexes_v1.md

// SchemaFilter is the standard WHERE clause that excludes PostgreSQL
// internal schemas from all schema metadata collectors.
const SchemaFilter = `
  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
  AND n.nspname NOT LIKE 'pg_temp_%'
  AND n.nspname NOT LIKE 'pg_toast_temp_%'`

// SchemaFilterDirect is the standard filter for views that expose
// schemaname directly (e.g., pg_indexes) without a pg_namespace join.
const SchemaFilterDirect = `
WHERE schemaname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
  AND schemaname NOT LIKE 'pg_temp_%'
  AND schemaname NOT LIKE 'pg_toast_temp_%'`

func init() {
	// pg_constraints_v1: constraint inventory, one row per constrained
	// column. Multi-column constraints emit multiple rows with the
	// same conname and sequential column_position values.
	//
	// Specification: specifications/collectors/pg_constraints_v1.md
	// Unblocks: FI-R010 through FI-R016 (Category 1 missing-FK-index detector)
	Register(QueryDef{
		ID:       "pg_constraints_v1",
		Category: "schema",
		SQL: `SELECT
			n.nspname AS schemaname,
			c.relname,
			con.conname,
			con.contype,
			pg_get_constraintdef(con.oid, true) AS condef,
			a.attname AS column_name,
			ord.ordinality::int AS column_position,
			c.relkind,
			COALESCE(s.n_live_tup, 0)::bigint AS n_live_tup,
			COALESCE(rc.relname, '') AS confrelname,
			COALESCE(rn.nspname, '') AS confschemaname
		FROM pg_constraint con
		JOIN pg_class c ON c.oid = con.conrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		CROSS JOIN LATERAL unnest(con.conkey) WITH ORDINALITY AS ord(attnum, ordinality)
		JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = ord.attnum
		LEFT JOIN pg_class rc ON rc.oid = con.confrelid
		LEFT JOIN pg_namespace rn ON rn.oid = rc.relnamespace
		LEFT JOIN pg_stat_user_tables s ON s.relid = c.oid
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		ORDER BY n.nspname, c.relname, con.conname, ord.ordinality`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        30 * time.Second,
		Cadence:        CadenceDaily,
	})

	// pg_indexes_v1: index definitions for all user-schema indexes.
	// The indexdef column contains the full CREATE INDEX statement,
	// needed to identify leading columns for composite indexes.
	//
	// Specification: specifications/collectors/pg_indexes_v1.md
	// Unblocks: FI-R014 (FK index suppression with leading column parsing)
	Register(QueryDef{
		ID:       "pg_indexes_v1",
		Category: "schema",
		SQL: `SELECT
			schemaname,
			tablename,
			indexname,
			indexdef,
			COALESCE(tablespace, '') AS tablespace
		FROM pg_indexes
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		  AND schemaname NOT LIKE 'pg_temp_%'
		  AND schemaname NOT LIKE 'pg_toast_temp_%'
		ORDER BY schemaname, tablename, indexname`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        30 * time.Second,
		Cadence:        CadenceDaily,
	})
}
