package pgqueries

import (
	"fmt"
	"sort"
)

var registry []QueryDef
var registryByID = map[string]*QueryDef{}

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
		out = append(out, q)
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
