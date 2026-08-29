package config

import (
	"net"
	"strings"
)

// NonCanonicalHostReason reports why a target's configured connect host is a
// NON-canonical database identity, or "" when the host is a canonical DNS
// hostname (#396).
//
// The Analyzer keys a database's identity on the composite
// (cluster_key, database_name) where cluster_key = lower(trim(host)):port —
// the connect host is echoed verbatim into the export's target_identity and is
// NOT reconciled to a DNS name. So collecting the SAME database under two
// different host strings (e.g. its DNS endpoint from one Signals instance and a
// loopback/tunnel address or a resolved IP from another) produces two distinct
// cluster_keys: the Analyzer recognizes the first and holds the second as an
// unknown identity, silently leaving those snapshots unprocessed.
//
// Signals cannot know which host string the database was registered under
// elsewhere, so it cannot unify them; the guard is advisory. A loopback host
// (localhost / 127.0.0.0/8 / ::1) or a bare IP literal is almost always a
// non-canonical identity that will split against the DNS endpoint the database
// is registered under, so we surface it loudly at startup. A DNS hostname is
// treated as canonical.
func NonCanonicalHostReason(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.Trim(h, "[]") // tolerate a bracketed IPv6 literal
	if h == "" {
		return "" // empty host is a separate required-field validation, not this guard
	}
	if h == "localhost" {
		return "loopback hostname \"localhost\""
	}
	if ip := net.ParseIP(h); ip != nil {
		if ip.IsLoopback() {
			return "loopback IP address"
		}
		return "bare IP literal (not a DNS hostname)"
	}
	return ""
}
