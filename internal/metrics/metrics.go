// Package metrics owns the Prometheus registry that powers the optional
// /metrics endpoint described in ARQ-SIGNALS-R079. The registry is
// dedicated (not the global default) so test code and embedded use
// don't see metrics they didn't ask for, and so we can guarantee that
// only the metrics defined here are ever exported.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Registry is the dedicated Prometheus registry for Arq Signals
// operational metrics. The /metrics endpoint serves from this registry
// only — it never exports the process, Go runtime, or default
// collectors. Operators monitoring the daemon process itself can do
// that via their orchestrator's existing tooling.
type Registry struct {
	reg                              *prometheus.Registry
	collectionCycles                 *prometheus.CounterVec
	collectionFailures               *prometheus.CounterVec
	collectionDuration               *prometheus.HistogramVec
	collectorsSucceeded              *prometheus.CounterVec
	collectorsFailed                 *prometheus.CounterVec
	collectorsSkipped                *prometheus.CounterVec
	exportRequests                   *prometheus.CounterVec
	exportFailures                   *prometheus.CounterVec
	exportDuration                   *prometheus.HistogramVec
	sqlitePersistenceFailures        prometheus.Counter
	lastSuccessfulCollectionTS       *prometheus.GaugeVec
	highSensitivityCollectorsEnabled prometheus.Gauge
}

// New constructs a Registry with all R079 metrics registered. The
// caller plugs the returned *prometheus.Registry into promhttp.
func New() *Registry {
	r := prometheus.NewRegistry()

	m := &Registry{
		reg: r,
		collectionCycles: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "arq_signal_collection_cycles_total",
				Help: "Per-target collection cycles completed, labelled by outcome.",
			},
			[]string{"target", "status"},
		),
		collectionFailures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "arq_signal_collection_failures_total",
				Help: "Per-target hard collection failures by reason category.",
			},
			[]string{"target", "reason"},
		),
		collectionDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "arq_signal_collection_duration_seconds",
				Help:    "Wall-clock duration of each per-target collection cycle.",
				Buckets: prometheus.ExponentialBuckets(0.05, 2, 10),
			},
			[]string{"target", "status"},
		),
		collectorsSucceeded: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "arq_signal_collectors_succeeded_total",
				Help: "Sum of per-cycle successful collector counts, by target.",
			},
			[]string{"target"},
		),
		collectorsFailed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "arq_signal_collectors_failed_total",
				Help: "Sum of per-cycle failed collector counts, by target and reason category.",
			},
			[]string{"target", "reason"},
		),
		collectorsSkipped: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "arq_signal_collectors_skipped_total",
				Help: "Sum of per-cycle skipped collector counts, by target and reason category.",
			},
			[]string{"target", "reason"},
		),
		exportRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "arq_signal_export_requests_total",
				Help: "Export requests received, labelled by outcome.",
			},
			[]string{"status"},
		),
		exportFailures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "arq_signal_export_failures_total",
				Help: "Export failures, labelled by error category (matches audit log error_category).",
			},
			[]string{"error_category"},
		),
		exportDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "arq_signal_export_duration_seconds",
				Help:    "Wall-clock duration of each export.",
				Buckets: prometheus.ExponentialBuckets(0.01, 3, 9),
			},
			[]string{"status"},
		),
		sqlitePersistenceFailures: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "arq_signal_sqlite_persistence_failures_total",
				Help: "InsertCollectionAtomic transaction rollbacks (R077).",
			},
		),
		lastSuccessfulCollectionTS: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "arq_signal_last_successful_collection_timestamp",
				Help: "Unix seconds of the most recent successful collection per target.",
			},
			[]string{"target"},
		),
		highSensitivityCollectorsEnabled: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "arq_signal_high_sensitivity_collectors_enabled",
				Help: "1 if signals.high_sensitivity_collectors_enabled is true, 0 otherwise (R075).",
			},
		),
	}

	r.MustRegister(
		m.collectionCycles,
		m.collectionFailures,
		m.collectionDuration,
		m.collectorsSucceeded,
		m.collectorsFailed,
		m.collectorsSkipped,
		m.exportRequests,
		m.exportFailures,
		m.exportDuration,
		m.sqlitePersistenceFailures,
		m.lastSuccessfulCollectionTS,
		m.highSensitivityCollectorsEnabled,
	)

	return m
}

// Gatherer returns the underlying prometheus.Gatherer for promhttp.
func (m *Registry) Gatherer() prometheus.Gatherer {
	return m.reg
}

// --- Recorders ---
//
// Recorders accept untyped strings for label values; callers supply
// the bounded enum value defined in R079. A nil receiver is treated
// as a no-op so call sites don't need to nil-check before incrementing
// — the metrics package is opt-in and a daemon running with
// metrics_enabled=false has a nil Registry.

func (m *Registry) ObserveCollection(target, status string, durationSeconds float64) {
	if m == nil {
		return
	}
	m.collectionCycles.WithLabelValues(target, status).Inc()
	m.collectionDuration.WithLabelValues(target, status).Observe(durationSeconds)
}

func (m *Registry) ObserveCollectionFailure(target, reason string) {
	if m == nil {
		return
	}
	m.collectionFailures.WithLabelValues(target, reason).Inc()
}

func (m *Registry) AddCollectorOutcomes(target string, succeeded int, failedByReason, skippedByReason map[string]int) {
	if m == nil {
		return
	}
	if succeeded > 0 {
		m.collectorsSucceeded.WithLabelValues(target).Add(float64(succeeded))
	}
	for reason, n := range failedByReason {
		if n > 0 {
			m.collectorsFailed.WithLabelValues(target, reason).Add(float64(n))
		}
	}
	for reason, n := range skippedByReason {
		if n > 0 {
			m.collectorsSkipped.WithLabelValues(target, reason).Add(float64(n))
		}
	}
}

func (m *Registry) RecordExport(status string, durationSeconds float64) {
	if m == nil {
		return
	}
	m.exportRequests.WithLabelValues(status).Inc()
	m.exportDuration.WithLabelValues(status).Observe(durationSeconds)
}

func (m *Registry) RecordExportFailure(errorCategory string) {
	if m == nil {
		return
	}
	m.exportFailures.WithLabelValues(errorCategory).Inc()
}

func (m *Registry) IncSQLitePersistenceFailure() {
	if m == nil {
		return
	}
	m.sqlitePersistenceFailures.Inc()
}

func (m *Registry) SetLastSuccessfulCollection(target string, unixSeconds float64) {
	if m == nil {
		return
	}
	m.lastSuccessfulCollectionTS.WithLabelValues(target).Set(unixSeconds)
}

func (m *Registry) SetHighSensitivityEnabled(enabled bool) {
	if m == nil {
		return
	}
	v := 0.0
	if enabled {
		v = 1.0
	}
	m.highSensitivityCollectorsEnabled.Set(v)
}
