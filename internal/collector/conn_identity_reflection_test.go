package collector

import (
	"reflect"
	"testing"

	"github.com/elevarq/signals/internal/config"
)

// TestConnIdentity_CoversEveryConnectionField is the robustness guard
// for SIGNALS-R100.1 (#328): it reflects over every field of
// config.TargetConfig and requires each to be EITHER represented in the
// derived connIdentity (so a change to it invalidates the pool on
// reload) OR explicitly listed as a known non-connection field (so its
// exclusion is a deliberate, reviewed decision).
//
// The whole point of deriving a connIdentity struct rather than a
// hand-written field list in sameConnection is that adding a new
// credential- or connection-affecting field to TargetConfig should fail
// THIS test until the author decides where it belongs — never be
// silently omitted from the reload comparison (the original #328 bug).
func TestConnIdentity_CoversEveryConnectionField(t *testing.T) {
	// Fields of TargetConfig that legitimately do NOT affect how the
	// collector dials/secures/authenticates a connection, and so are
	// intentionally excluded from connIdentity. A change to any of these
	// alone must NOT drop the pool.
	nonConnection := map[string]string{
		"Name":       "target identity/key, not a dial parameter",
		"Enabled":    "collection gate, not a connection parameter",
		"Collectors": "R098 sensitivity profile, not a connection parameter",
		// MaxCacheTTLS is the raw YAML duration string folded into the
		// typed MaxCacheTTL by parseDurations before reload ever runs;
		// MaxCacheTTL (the resolved value) is what connIdentity compares.
		"MaxCacheTTLS": "raw YAML form of MaxCacheTTL; resolved value is compared",
	}

	identityFields := map[string]struct{}{}
	it := reflect.TypeOf(connIdentity{})
	for i := 0; i < it.NumField(); i++ {
		identityFields[it.Field(i).Name] = struct{}{}
	}

	tt := reflect.TypeOf(config.TargetConfig{})
	for i := 0; i < tt.NumField(); i++ {
		name := tt.Field(i).Name
		_, inIdentity := identityFields[name]
		_, excluded := nonConnection[name]
		switch {
		case inIdentity && excluded:
			t.Errorf("TargetConfig.%s is both in connIdentity and the non-connection allowlist — pick one", name)
		case !inIdentity && !excluded:
			t.Errorf("TargetConfig.%s is neither in connIdentity nor the non-connection allowlist. "+
				"If it affects dialing/TLS/credential selection/content/caching, add it to connIdentity "+
				"(and connectionIdentity + the R100.1 spec + the reload test table). If it does not affect "+
				"connections, add it to nonConnection with a justification.", name)
		}
	}
}
