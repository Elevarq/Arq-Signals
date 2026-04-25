package pgqueries

// catalog_pg14.go — version-specific overrides for PostgreSQL 14.
//
// PG 14 is the oldest supported major (R081). No collector currently
// requires a PG 14-specific SQL variant: every collector that's eligible
// for PG 14 uses the version-agnostic SQL from the default catalog. If
// a future PG 14 schema-deprecation forces a fork, register it here via
// RegisterOverride(14, "<query_id>", "<sql>").
//
// This file exists to preserve the per-major file layout described in
// the spec and to give the per-version catalog list a consistent
// shape: catalog_pg14.go … catalog_pg18.go each define the exception
// set for their major.
func init() {
	// no overrides
}
