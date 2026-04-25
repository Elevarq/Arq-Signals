package pgqueries

import (
	"fmt"
	"sort"
)

var registry []QueryDef
var registryByID = map[string]*QueryDef{}

// overrideRegistry holds version-specific SQL overrides. Keyed first by
// PG major (14, 15, 16, 17, 18, …), then by logical query ID. R081:
// catalog_pgN.go init() functions populate this map; Filter() consults
// it when resolving the effective SQL for the connected server's major.
//
// A logical ID's *default* SQL lives in the version-agnostic catalog
// files (`catalog.go`, `catalog_io.go`, etc.). Overrides are exceptions
// for collectors whose underlying catalog view changed between PG
// majors (e.g. pg_stat_io's column rename in PG 18). Logical IDs stay
// stable across versions — consumers see the same query_id, only the
// SQL underneath differs.
var overrideRegistry = map[int]map[string]string{}

// validCadences is the set of allowed Cadence values (zero included as default).
var validCadences = map[Cadence]bool{
	0:             true,
	Cadence5m:     true,
	Cadence15m:    true,
	Cadence1h:     true,
	Cadence6h:     true,
	CadenceDaily:  true,
	CadenceWeekly: true,
}

// Register adds a query definition to the global registry.
// It panics if the query fails lint, has a duplicate ID, or uses an invalid cadence.
// Must be called from init().
func Register(q QueryDef) {
	if err := LintQuery(q.SQL); err != nil {
		panic(fmt.Sprintf("pgqueries.Register(%q): lint failed: %v", q.ID, err))
	}
	if _, exists := registryByID[q.ID]; exists {
		panic(fmt.Sprintf("pgqueries.Register(%q): duplicate ID", q.ID))
	}
	if !validCadences[q.Cadence] {
		panic(fmt.Sprintf("pgqueries.Register(%q): invalid cadence %v", q.ID, q.Cadence))
	}
	registry = append(registry, q)
	registryByID[q.ID] = &registry[len(registry)-1]
}

// RegisterOverride records SQL that replaces the default for a logical
// collector when the connected target runs the given PG major. R081:
// called from catalog_pgN.go init(); the SQL is lint-checked at
// registration time exactly like the default registry. Overriding a
// non-existent logical ID, or registering a duplicate override for the
// same (major, id) pair, panics — both conditions indicate a coding
// error in the per-major catalog files.
func RegisterOverride(major int, id, sql string) {
	if _, exists := registryByID[id]; !exists {
		panic(fmt.Sprintf("pgqueries.RegisterOverride(major=%d, id=%q): unknown logical id; must be registered in the default catalog first", major, id))
	}
	if err := LintQuery(sql); err != nil {
		panic(fmt.Sprintf("pgqueries.RegisterOverride(major=%d, id=%q): lint failed: %v", major, id, err))
	}
	if overrideRegistry[major] == nil {
		overrideRegistry[major] = map[string]string{}
	}
	if _, dup := overrideRegistry[major][id]; dup {
		panic(fmt.Sprintf("pgqueries.RegisterOverride(major=%d, id=%q): duplicate override", major, id))
	}
	overrideRegistry[major][id] = sql
}

// resolveSQL returns the effective SQL for `id` against the connected
// server's major. If a version-specific override exists, that wins;
// otherwise the default SQL from Register is used.
func resolveSQL(id string, major int, defaultSQL string) string {
	if m := overrideRegistry[major]; m != nil {
		if sql, ok := m[id]; ok {
			return sql
		}
	}
	return defaultSQL
}

// HasOverride reports whether (major, id) has a registered override.
// Used by tests; not on the hot path.
func HasOverride(major int, id string) bool {
	if m := overrideRegistry[major]; m != nil {
		_, ok := m[id]
		return ok
	}
	return false
}

// All returns all registered queries sorted by ID.
func All() []QueryDef {
	out := make([]QueryDef, len(registry))
	copy(out, registry)
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

// Filter returns queries eligible for the given PG version and extensions.
// High-sensitivity queries are excluded unless p.HighSensitivityEnabled is
// set; the collector emits a skipped/config_disabled status for those
// separately so operators can see the gate is active.
//
// R081: each returned QueryDef has its `SQL` field already resolved
// against `overrideRegistry[p.PGMajorVersion]`. Callers do not need to
// know whether they're getting default or version-specific SQL; the
// logical ID is stable either way.
func Filter(p FilterParams) []QueryDef {
	extSet := make(map[string]bool, len(p.Extensions))
	for _, e := range p.Extensions {
		extSet[e] = true
	}

	var out []QueryDef
	for _, q := range registry {
		if q.MinPGVersion > 0 && p.PGMajorVersion < q.MinPGVersion {
			continue
		}
		if q.RequiresExtension != "" && !extSet[q.RequiresExtension] {
			continue
		}
		if q.HighSensitivity && !p.HighSensitivityEnabled {
			continue
		}
		// Copy and swap SQL with the resolved (override or default).
		resolved := q
		resolved.SQL = resolveSQL(q.ID, p.PGMajorVersion, q.SQL)
		out = append(out, resolved)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

// HighSensitivityIDs returns the IDs of all registered high-sensitivity
// queries that are eligible for the given PG version and extensions but
// are gated off by HighSensitivityEnabled. Used to emit
// status=skipped/reason=config_disabled entries in collector_status.json.
func HighSensitivityIDs(p FilterParams) []string {
	if p.HighSensitivityEnabled {
		return nil
	}
	extSet := make(map[string]bool, len(p.Extensions))
	for _, e := range p.Extensions {
		extSet[e] = true
	}
	var out []string
	for _, q := range registry {
		if !q.HighSensitivity {
			continue
		}
		if q.MinPGVersion > 0 && p.PGMajorVersion < q.MinPGVersion {
			continue
		}
		if q.RequiresExtension != "" && !extSet[q.RequiresExtension] {
			continue
		}
		out = append(out, q.ID)
	}
	sort.Strings(out)
	return out
}

// ByID returns the query with the given ID, or nil if not found.
func ByID(id string) *QueryDef {
	return registryByID[id]
}
