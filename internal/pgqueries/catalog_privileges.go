package pgqueries

import "time"

// Object-level and default access privileges (#363; feeds Analyzer #1757).
//
// These collectors carry the actual ACLs so a downstream security-posture
// analysis (Analyzer #1757) can flag over-broad privileges — grants to
// PUBLIC, or broad privileges on the `public` schema. `login_roles_v1` /
// `pg_role_capabilities_v1` already describe role *capabilities*; these add
// the object/default *grants* that were missing.
//
// Both normalise the `aclitem[]` ACL arrays via `aclexplode()` into one
// (grantee, privilege, grantable) row per grant. A grantee OID of 0 is
// PUBLIC (rendered as the literal "PUBLIC"); every other OID is resolved to
// its role name with `pg_get_userbyid`. Only catalog privilege *metadata*
// is read — no object data — so no extra grant beyond the default
// `pg_monitor` collection role is required, and role names are the same
// class of already-emitted, non-sensitive catalog data as `login_roles_v1`
// (no redaction).
func init() {
	// Object-level privileges: table/view/matview/sequence/foreign-table
	// (pg_class.relacl), schema (pg_namespace.nspacl), and function
	// (pg_proc.proacl) grants. A NULL acl means "owner default, no explicit
	// grants" and produces no rows — the presence of an explicit row for a
	// broad grantee (PUBLIC / a broad role) is exactly the signal the
	// consumer wants. The `public` schema carries a default grant on every
	// supported major, so this collector emits at least those rows on any
	// database.
	Register(QueryDef{
		ID:       "object_privileges_v1",
		Category: "security",
		SQL: `WITH obj AS (
	SELECT
		CASE c.relkind
			WHEN 'r' THEN 'table'
			WHEN 'p' THEN 'partitioned_table'
			WHEN 'v' THEN 'view'
			WHEN 'm' THEN 'materialized_view'
			WHEN 'S' THEN 'sequence'
			WHEN 'f' THEN 'foreign_table'
			ELSE c.relkind::text
		END AS object_kind,
		n.nspname AS schema_name,
		c.relname AS object_name,
		c.relacl  AS acl
	FROM pg_class c
	JOIN pg_namespace n ON n.oid = c.relnamespace
	WHERE c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')
	  AND c.relacl IS NOT NULL
	  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
	  AND n.nspname NOT LIKE 'pg_temp_%'
	  AND n.nspname NOT LIKE 'pg_toast_temp_%'
	UNION ALL
	SELECT 'schema', n.nspname, n.nspname, n.nspacl
	FROM pg_namespace n
	WHERE n.nspacl IS NOT NULL
	  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
	  AND n.nspname NOT LIKE 'pg_temp_%'
	  AND n.nspname NOT LIKE 'pg_toast_temp_%'
	UNION ALL
	SELECT 'function', n.nspname, p.proname, p.proacl
	FROM pg_proc p
	JOIN pg_namespace n ON n.oid = p.pronamespace
	WHERE p.proacl IS NOT NULL
	  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
	  AND n.nspname NOT LIKE 'pg_temp_%'
	  AND n.nspname NOT LIKE 'pg_toast_temp_%'
)
SELECT
	obj.object_kind,
	obj.schema_name,
	obj.object_name,
	COALESCE(pg_get_userbyid(NULLIF(a.grantee, 0)), 'PUBLIC') AS grantee,
	a.privilege_type,
	a.is_grantable,
	pg_get_userbyid(a.grantor) AS grantor
FROM obj
CROSS JOIN LATERAL aclexplode(obj.acl) AS a
ORDER BY obj.object_kind, obj.schema_name, obj.object_name, grantee, a.privilege_type, a.is_grantable`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionLong,
		Timeout:        10 * time.Second,
		Cadence:        Cadence6h,
	})

	// Default privileges: pg_default_acl.defaclacl — what FUTURE objects
	// created by `defaclrole` (optionally scoped to a schema) will inherit.
	// Empty on a fresh database; rows appear only once someone runs
	// ALTER DEFAULT PRIVILEGES, which is exactly the configuration the
	// consumer needs to reason about. `schema_name` is empty for a global
	// (non-schema-scoped) default.
	Register(QueryDef{
		ID:       "default_privileges_v1",
		Category: "security",
		SQL: `SELECT
	pg_get_userbyid(d.defaclrole) AS for_role,
	COALESCE(n.nspname, '') AS schema_name,
	CASE d.defaclobjtype
		WHEN 'r' THEN 'table'
		WHEN 'S' THEN 'sequence'
		WHEN 'f' THEN 'function'
		WHEN 'T' THEN 'type'
		WHEN 'n' THEN 'schema'
		ELSE d.defaclobjtype::text
	END AS object_kind,
	COALESCE(pg_get_userbyid(NULLIF(a.grantee, 0)), 'PUBLIC') AS grantee,
	a.privilege_type,
	a.is_grantable
FROM pg_default_acl d
LEFT JOIN pg_namespace n ON n.oid = d.defaclnamespace
CROSS JOIN LATERAL aclexplode(d.defaclacl) AS a
ORDER BY for_role, schema_name, object_kind, grantee, a.privilege_type, a.is_grantable`,
		ResultKind:     ResultRowset,
		RetentionClass: RetentionLong,
		Timeout:        10 * time.Second,
		Cadence:        Cadence6h,
	})
}
