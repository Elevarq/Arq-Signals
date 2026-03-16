package pgqueries

import "time"

// Schema Metadata Collectors
//
// These collectors extract structural schema metadata on a slow
// cadence (default 24h). They focus on non-system objects only.
//
// Phase 1: pg_constraints_v1, pg_indexes_v1, pg_stats_v1, pg_columns_v1
// Phase 2: pg_schemas_v1, ...
//
// Specifications: specifications/collectors/*.md

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

	// pg_stats_v1: column-level planner statistics for cardinality
	// and correlation analysis. Deliberately excludes most_common_vals,
	// histogram_bounds, and other columns that contain data samples.
	//
	// Specification: specifications/collectors/pg_stats_v1.md
	// Unblocks: FI-R012 (n_distinct cardinality), FI-R052 (correlation)
	Register(QueryDef{
		ID:       "pg_stats_v1",
		Category: "schema",
		SQL: `SELECT
			schemaname,
			tablename,
			attname,
			n_distinct,
			correlation,
			null_frac,
			avg_width
		FROM pg_stats
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		  AND schemaname NOT LIKE 'pg_temp_%'
		  AND schemaname NOT LIKE 'pg_toast_temp_%'
		ORDER BY schemaname, tablename, attname`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        30 * time.Second,
		Cadence:        CadenceDaily,
	})

	// pg_columns_v1: column inventory with data types for user-schema
	// relations. Uses pg_attribute + pg_class + pg_namespace + pg_attrdef
	// with format_type() for human-readable type names. Excludes system
	// columns (attnum <= 0) and dropped columns. Default expression
	// text is NOT emitted — only the boolean has_default.
	//
	// Specification: specifications/collectors/pg_columns_v1.md
	Register(QueryDef{
		ID:       "pg_columns_v1",
		Category: "schema",
		SQL: `SELECT
			n.nspname AS schemaname,
			c.relname,
			a.attname,
			a.attnum,
			format_type(a.atttypid, a.atttypmod) AS typname,
			NOT a.attnotnull AS is_nullable,
			d.adrelid IS NOT NULL AS has_default,
			a.attlen
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE a.attnum > 0
		  AND NOT a.attisdropped
		  AND c.relkind IN ('r', 'p', 'v', 'm', 'f')
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		ORDER BY n.nspname, c.relname, a.attnum`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        30 * time.Second,
		Cadence:        CadenceDaily,
	})

	// --- Phase 2: Schema snapshot foundation ---

	// pg_schemas_v1: schema (namespace) inventory with ownership.
	// Provides namespace context for all other schema collectors.
	//
	// Specification: specifications/collectors/pg_schemas_v1.md
	Register(QueryDef{
		ID:       "pg_schemas_v1",
		Category: "schema",
		SQL: `SELECT
			n.nspname,
			r.rolname AS nspowner,
			n.nspname = 'public' AS is_default
		FROM pg_namespace n
		JOIN pg_roles r ON r.oid = n.nspowner
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		ORDER BY n.nspname`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionLong,
		Timeout:        10 * time.Second,
		Cadence:        CadenceDaily,
	})

	// pg_views_v1: view inventory (inventory mode — no definition text).
	// Lists all user-schema views with owner. Definition text is
	// excluded by default for safety; use pg_views_definitions_v1
	// when definition/hash_only mode is needed.
	//
	// Specification: specifications/collectors/pg_views_v1.md
	Register(QueryDef{
		ID:       "pg_views_v1",
		Category: "schema",
		SQL: `SELECT
			schemaname,
			viewname,
			viewowner
		FROM pg_views
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		  AND schemaname NOT LIKE 'pg_temp_%'
		  AND schemaname NOT LIKE 'pg_toast_temp_%'
		ORDER BY schemaname, viewname`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        10 * time.Second,
		Cadence:        CadenceDaily,
	})

	// pg_views_definitions_v1: view inventory with definition text
	// (definition mode). Includes all inventory columns plus the
	// full view SQL from pg_get_viewdef(). Disabled by default in
	// typical configurations; enabled when schema drift detection
	// or documentation is needed.
	//
	// For hash_only mode, the Arq Signals runtime computes SHA-256
	// of the definition column application-side before persistence,
	// then strips the raw text. No pgcrypto dependency.
	//
	// Specification: specifications/collectors/pg_views_v1.md
	Register(QueryDef{
		ID:       "pg_views_definitions_v1",
		Category: "schema",
		SQL: `SELECT
			v.schemaname,
			v.viewname,
			v.viewowner,
			pg_get_viewdef(c.oid, true) AS definition
		FROM pg_views v
		JOIN pg_class c ON c.relname = v.viewname
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = v.schemaname
		WHERE v.schemaname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		  AND v.schemaname NOT LIKE 'pg_temp_%'
		  AND v.schemaname NOT LIKE 'pg_toast_temp_%'
		ORDER BY v.schemaname, v.viewname`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        30 * time.Second,
		Cadence:        CadenceDaily,
	})

	// pg_matviews_v1: materialized view inventory (inventory mode).
	// Lists all user-schema matviews with owner, populated status,
	// and index presence. Definition text excluded by default.
	//
	// Specification: specifications/collectors/pg_matviews_v1.md
	Register(QueryDef{
		ID:       "pg_matviews_v1",
		Category: "schema",
		SQL: `SELECT
			schemaname,
			matviewname,
			matviewowner,
			ispopulated,
			hasindexes
		FROM pg_matviews
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		  AND schemaname NOT LIKE 'pg_temp_%'
		  AND schemaname NOT LIKE 'pg_toast_temp_%'
		ORDER BY schemaname, matviewname`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        10 * time.Second,
		Cadence:        CadenceDaily,
	})

	// pg_matviews_definitions_v1: materialized view inventory with
	// definition text (definition mode). Includes inventory columns
	// plus full matview SQL from pg_get_viewdef().
	//
	// Specification: specifications/collectors/pg_matviews_v1.md
	Register(QueryDef{
		ID:       "pg_matviews_definitions_v1",
		Category: "schema",
		SQL: `SELECT
			m.schemaname,
			m.matviewname,
			m.matviewowner,
			m.ispopulated,
			m.hasindexes,
			pg_get_viewdef(c.oid, true) AS definition
		FROM pg_matviews m
		JOIN pg_class c ON c.relname = m.matviewname
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = m.schemaname
		WHERE m.schemaname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		  AND m.schemaname NOT LIKE 'pg_temp_%'
		  AND m.schemaname NOT LIKE 'pg_toast_temp_%'
		ORDER BY m.schemaname, m.matviewname`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionMedium,
		Timeout:        30 * time.Second,
		Cadence:        CadenceDaily,
	})
}
