package pgqueries

import "time"

// ResultKind describes the shape of a query's result.
type ResultKind string

const (
	ResultScalar ResultKind = "scalar"
	ResultRowset ResultKind = "rowset"
)

// RetentionClass hints how long results should be kept.
type RetentionClass string

const (
	RetentionShort  RetentionClass = "short"
	RetentionMedium RetentionClass = "medium"
	RetentionLong   RetentionClass = "long"
)

// Cadence controls how often a query should be executed.
type Cadence time.Duration

const (
	Cadence5m     Cadence = Cadence(5 * time.Minute)
	Cadence15m    Cadence = Cadence(15 * time.Minute)
	Cadence1h     Cadence = Cadence(1 * time.Hour)
	Cadence6h     Cadence = Cadence(6 * time.Hour)
	CadenceDaily  Cadence = Cadence(24 * time.Hour)
	CadenceWeekly Cadence = Cadence(7 * 24 * time.Hour)
)

// CadenceDefault is used when a query has zero-value cadence.
const CadenceDefault = Cadence1h

// Duration returns the cadence as a time.Duration.
// Returns CadenceDefault if zero.
func (c Cadence) Duration() time.Duration {
	if c == 0 {
		return time.Duration(CadenceDefault)
	}
	return time.Duration(c)
}

// String returns a human-readable label for the cadence.
func (c Cadence) String() string {
	if c == 0 {
		c = CadenceDefault
	}
	switch c {
	case Cadence5m:
		return "5m"
	case Cadence15m:
		return "15m"
	case Cadence1h:
		return "1h"
	case Cadence6h:
		return "6h"
	case CadenceDaily:
		return "24h"
	case CadenceWeekly:
		return "7d"
	default:
		return time.Duration(c).String()
	}
}

// QueryDef defines a single versioned SQL query.
type QueryDef struct {
	ID                string
	Category          string
	RequiresExtension string
	SQL               string
	MinPGVersion      int
	ResultKind        ResultKind
	RetentionClass    RetentionClass
	Timeout           time.Duration
	Cadence           Cadence
	// HighSensitivity flags collectors that emit application-authored SQL
	// text (view/matview/trigger/function definitions). Per R075 these
	// run only when the operator opts in.
	HighSensitivity bool
}

// FilterParams controls which queries are eligible for a given target.
type FilterParams struct {
	PGMajorVersion         int
	Extensions             []string
	HighSensitivityEnabled bool
}
