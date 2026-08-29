package config

import "testing"

// TestScheduledExportEnabled_403 — the destination is the switch: a non-empty
// export_dest enables the scheduled directory-push unless export_on_collect is
// explicitly false (the opt-out). No dest ⇒ pull-only (default).
func TestScheduledExportEnabled_403(t *testing.T) {
	p := func(b bool) *bool { return &b }
	cases := []struct {
		name      string
		dest      string
		onCollect *bool
		want      bool
	}{
		{"no dest, unset -> pull-only (default)", "", nil, false},
		{"no dest, on_collect=true -> still no push (nowhere to write)", "", p(true), false},
		{"dest set, unset -> push (dest is the switch)", "/exports", nil, true},
		{"dest set, on_collect=true -> push", "/exports", p(true), true},
		{"dest set, on_collect=false -> opt-out, no push", "/exports", p(false), false},
	}
	for _, c := range cases {
		s := SignalsConfig{ExportDest: c.dest, ExportOnCollect: c.onCollect}
		if got := s.ScheduledExportEnabled(); got != c.want {
			t.Errorf("%s: ScheduledExportEnabled() = %v, want %v", c.name, got, c.want)
		}
	}
}
