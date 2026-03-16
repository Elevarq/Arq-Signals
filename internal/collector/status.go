package collector

import (
	"sort"
	"time"
)

// CollectorStatus records the execution outcome of a single collector.
//
// Specification: specifications/collector_status.md
type CollectorStatus struct {
	ID          string `json:"id"`
	Attempted   bool   `json:"attempted"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
	Detail      string `json:"detail"`
	RowCount    int    `json:"row_count"`
	DurationMS  int    `json:"duration_ms"`
	CollectedAt string `json:"collected_at"`
}

// CollectorStatusFile is the top-level structure for collector_status.json.
type CollectorStatusFile struct {
	SchemaVersion string             `json:"schema_version"`
	CollectedAt   string             `json:"collected_at"`
	Collectors    []CollectorStatus  `json:"collectors"`
}

// Sort orders collectors by ID for deterministic output.
func (f *CollectorStatusFile) Sort() {
	sort.Slice(f.Collectors, func(i, j int) bool {
		return f.Collectors[i].ID < f.Collectors[j].ID
	})
}

// NewSuccessStatus creates a status entry for a successful collection.
func NewSuccessStatus(id string, rowCount, durationMS int, collectedAt time.Time) CollectorStatus {
	return CollectorStatus{
		ID:          id,
		Attempted:   true,
		Status:      "success",
		Reason:      "",
		Detail:      "",
		RowCount:    rowCount,
		DurationMS:  durationMS,
		CollectedAt: collectedAt.UTC().Format(time.RFC3339),
	}
}

// NewSkippedStatus creates a status entry for a skipped collector.
func NewSkippedStatus(id, reason, detail string) CollectorStatus {
	return CollectorStatus{
		ID:          id,
		Attempted:   false,
		Status:      "skipped",
		Reason:      reason,
		Detail:      detail,
		RowCount:    0,
		DurationMS:  0,
		CollectedAt: "",
	}
}

// NewFailedStatus creates a status entry for a failed collector.
func NewFailedStatus(id, reason, detail string, durationMS int, collectedAt time.Time) CollectorStatus {
	return CollectorStatus{
		ID:          id,
		Attempted:   true,
		Status:      "failed",
		Reason:      reason,
		Detail:      detail,
		RowCount:    0,
		DurationMS:  durationMS,
		CollectedAt: collectedAt.UTC().Format(time.RFC3339),
	}
}
