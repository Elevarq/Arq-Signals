package config

import "testing"

// TestNonCanonicalHostReason_396 covers the connect-host canonicity guard:
// loopback hosts and bare IP literals are non-canonical (they split against the
// DNS endpoint a database is registered under); DNS hostnames are canonical.
func TestNonCanonicalHostReason_396(t *testing.T) {
	canonical := []string{
		"elevarq-demo-timeseries.abc123.us-east-1.rds.amazonaws.com",
		"db",
		"postgres",
		"my-host.internal",
		"pg.svc.cluster.local",
		"", // empty is a separate required-field check, not this guard
		"  ",
	}
	for _, h := range canonical {
		if r := NonCanonicalHostReason(h); r != "" {
			t.Errorf("NonCanonicalHostReason(%q) = %q, want \"\" (canonical)", h, r)
		}
	}

	nonCanonical := []string{
		"localhost", "LOCALHOST", " localhost ",
		"127.0.0.1", "127.0.0.5", "127.1.2.3",
		"::1", "[::1]",
		"10.0.0.5", "192.168.1.10", "172.16.0.1", "8.8.8.8",
		"2001:db8::1", "[2001:db8::1]",
	}
	for _, h := range nonCanonical {
		if r := NonCanonicalHostReason(h); r == "" {
			t.Errorf("NonCanonicalHostReason(%q) = \"\", want a non-canonical reason", h)
		}
	}
}
