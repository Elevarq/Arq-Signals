package export

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/elevarq/arq-signals/internal/collector"
	"github.com/elevarq/arq-signals/internal/db"
	"github.com/elevarq/arq-signals/internal/safety"
	"github.com/elevarq/arq-signals/snapshot"
)

// Options controls what data is included in the export.
type Options struct {
	TargetID int64
	Since    string
	Until    string
}

// CollectorStatusSchemaVersion is the schema version embedded in
// collector_status.json. Bumped independently of the snapshot schema so
// auditors can pin tooling to a specific status format.
const CollectorStatusSchemaVersion = "1"

// Builder creates a ZIP export of collected data.
type Builder struct {
	store                            *db.DB
	instanceID                       string
	unsafeMode                       bool
	unsafeReasonsFunc                func() []string
	collectorStatus                  *collector.CollectorStatusFile
	highSensitivityCollectorsEnabled bool
}

// NewBuilder creates a new export Builder.
func NewBuilder(store *db.DB, instanceID string) *Builder {
	return &Builder{store: store, instanceID: instanceID}
}

// SetCollectorStatus provides collector execution status data for
// inclusion in the export ZIP as collector_status.json.
func (b *Builder) SetCollectorStatus(status *collector.CollectorStatusFile) {
	b.collectorStatus = status
}

// SetUnsafeMode marks the export metadata as collected in unsafe mode.
func (b *Builder) SetUnsafeMode(reasonsFunc func() []string) {
	b.unsafeMode = true
	b.unsafeReasonsFunc = reasonsFunc
}

// SetHighSensitivityCollectorsEnabled records the daemon-wide R075 gate
// state so it can be embedded in export metadata. Auditors use this to
// determine whether application-authored SQL definition text could be
// present in the export without parsing the body.
func (b *Builder) SetHighSensitivityCollectorsEnabled(enabled bool) {
	b.highSensitivityCollectorsEnabled = enabled
}

// WriteTo writes the ZIP export to the given writer.
func (b *Builder) WriteTo(w io.Writer, opts Options) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	if err := b.writeMetadata(zw, opts); err != nil {
		return fmt.Errorf("write metadata.json: %w", err)
	}

	if err := b.writeCollectorStatus(zw, opts); err != nil {
		return fmt.Errorf("write collector_status.json: %w", err)
	}

	if err := b.writeSnapshots(zw, opts); err != nil {
		return fmt.Errorf("write snapshots.ndjson: %w", err)
	}

	if err := b.writeQueryCatalog(zw); err != nil {
		return fmt.Errorf("write query_catalog.json: %w", err)
	}

	if err := b.writeQueryRuns(zw, opts); err != nil {
		return fmt.Errorf("write query_runs.ndjson: %w", err)
	}

	if err := b.writeQueryResults(zw, opts); err != nil {
		return fmt.Errorf("write query_results.ndjson: %w", err)
	}

	return nil
}

func (b *Builder) writeMetadata(zw *zip.Writer, opts Options) error {
	f, err := zw.Create("metadata.json")
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	data := map[string]any{
		"schema_version":                      snapshot.SchemaVersion,
		"collector_status_schema_version":     CollectorStatusSchemaVersion,
		"instance_id":                         b.instanceID,
		"arq_signals_version":                 safety.Version,
		"collector_version":                   safety.Version, // legacy alias
		"collector_commit":                    safety.Commit,
		"generated_at":                        now,
		"collected_at":                        now, // legacy alias
		"unsafe_mode":                         b.unsafeMode,
		"high_sensitivity_collectors_enabled": b.highSensitivityCollectorsEnabled,
	}
	if opts.TargetID > 0 {
		if name, err := b.store.GetTargetName(opts.TargetID); err == nil && name != "" {
			data["target_name"] = name
		}
	}
	if b.unsafeMode && b.unsafeReasonsFunc != nil {
		reasons := b.unsafeReasonsFunc()
		if len(reasons) > 0 {
			data["unsafe_reasons"] = reasons
		}
	}
	return json.NewEncoder(f).Encode(data)
}

func (b *Builder) writeCollectorStatus(zw *zip.Writer, opts Options) error {
	f, err := zw.Create("collector_status.json")
	if err != nil {
		return err
	}

	// Target-scoped: build status from query runs for that target (MTE-R004)
	if opts.TargetID > 0 {
		runs, err := b.store.GetQueryRunsByTarget(opts.TargetID, opts.Since, opts.Until)
		if err != nil {
			return err
		}

		targetName := b.resolveTargetName(opts.TargetID)
		statuses := collector.BuildStatusFromRuns(runs)

		file := collector.CollectorStatusFile{
			SchemaVersion: CollectorStatusSchemaVersion,
			TargetName:    targetName,
			CollectedAt:   time.Now().UTC().Format(time.RFC3339),
			Collectors:    statuses,
		}
		file.Sort()
		return json.NewEncoder(f).Encode(file)
	}

	// Instance-level: use the explicitly-supplied status if any.
	if b.collectorStatus != nil {
		b.collectorStatus.Sort()
		return json.NewEncoder(f).Encode(b.collectorStatus)
	}

	// Unscoped export with no caller-supplied status: synthesise from
	// the query_runs table (Codex post-0.3.1 H-002). The legacy
	// behaviour was to write an empty collectors[] array even when
	// matching query runs existed, which made auditors believe nothing
	// had been collected. Synthesising from runs keeps the file
	// non-empty whenever the cycles actually persisted runs. We make
	// no attempt to dedupe across targets — collectors that ran
	// against multiple targets appear once per target/run.
	runs, err := b.store.GetAllQueryRuns(opts.Since, opts.Until)
	if err != nil {
		return err
	}
	file := collector.CollectorStatusFile{
		SchemaVersion: CollectorStatusSchemaVersion,
		CollectedAt:   time.Now().UTC().Format(time.RFC3339),
		Collectors:    collector.BuildStatusFromRuns(runs),
	}
	if file.Collectors == nil {
		file.Collectors = []collector.CollectorStatus{}
	}
	file.Sort()
	return json.NewEncoder(f).Encode(file)
}

func (b *Builder) resolveTargetName(targetID int64) string {
	name, err := b.store.GetTargetName(targetID)
	if err != nil || name == "" {
		return fmt.Sprintf("target-%d", targetID)
	}
	return name
}

func (b *Builder) writeQueryCatalog(zw *zip.Writer) error {
	f, err := zw.Create("query_catalog.json")
	if err != nil {
		return err
	}

	catalog, err := b.store.GetQueryCatalog()
	if err != nil {
		return err
	}
	return json.NewEncoder(f).Encode(catalog)
}

// writeQueryRuns filters by target when TargetID is set (MTE-R001).
func (b *Builder) writeQueryRuns(zw *zip.Writer, opts Options) error {
	f, err := zw.Create("query_runs.ndjson")
	if err != nil {
		return err
	}

	var runs []db.QueryRun
	if opts.TargetID > 0 {
		runs, err = b.store.GetQueryRunsByTarget(opts.TargetID, opts.Since, opts.Until)
	} else {
		runs, err = b.store.GetAllQueryRuns(opts.Since, opts.Until)
	}
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	for _, r := range runs {
		row := map[string]any{
			"id":           r.ID,
			"target_id":    r.TargetID,
			"snapshot_id":  r.SnapshotID,
			"query_id":     r.QueryID,
			"collected_at": r.CollectedAt,
			"pg_version":   r.PGVersion,
			"duration_ms":  r.DurationMS,
			"row_count":    r.RowCount,
			"error":        r.Error,
		}
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	return nil
}

// writeQueryResults filters by target when TargetID is set (MTE-R002).
func (b *Builder) writeQueryResults(zw *zip.Writer, opts Options) error {
	f, err := zw.Create("query_results.ndjson")
	if err != nil {
		return err
	}

	var runs []db.QueryRun
	if opts.TargetID > 0 {
		runs, err = b.store.GetQueryRunsByTarget(opts.TargetID, opts.Since, opts.Until)
	} else {
		runs, err = b.store.GetAllQueryRuns(opts.Since, opts.Until)
	}
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	for _, r := range runs {
		// Skipped and failed runs legitimately have no payload.
		// Anything not status='success' (with the legacy fallback for
		// pre-status rows where status is empty and error is empty)
		// is allowed to be silently absent.
		isSuccess := r.Status == "success" || (r.Status == "" && r.Error == "")
		if !isSuccess {
			continue
		}

		res, err := b.store.GetQueryResultByRunID(r.ID)
		if err != nil {
			return fmt.Errorf("read result for run %s: %w", r.ID, err)
		}
		// A successful run that has no result payload is a data
		// integrity failure: InsertCollectionAtomic guarantees the
		// pair lands together, so a missing partner means the row
		// was deleted out of band or the storage corrupted. Codex
		// post-0.3.1 M-001 — fail the export instead of silently
		// dropping the row, otherwise audits believe collection
		// produced no data when it actually did.
		if res == nil {
			return fmt.Errorf("missing result payload for successful run %s (query_id=%s)", r.ID, r.QueryID)
		}
		decoded, err := db.DecodeNDJSON(res.Payload, res.Compressed)
		if err != nil {
			return fmt.Errorf("decode result for run %s (query_id=%s): %w", r.ID, r.QueryID, err)
		}
		row := map[string]any{
			"run_id":  r.ID,
			"payload": decoded,
		}
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	return nil
}

func (b *Builder) writeSnapshots(zw *zip.Writer, opts Options) error {
	f, err := zw.Create("snapshots.ndjson")
	if err != nil {
		return err
	}

	var snaps []db.Snapshot
	if opts.TargetID > 0 {
		snaps, err = b.store.GetSnapshotsByTarget(opts.TargetID, opts.Since, opts.Until)
	} else {
		snaps, err = b.store.GetAllSnapshots(opts.Since, opts.Until)
	}
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	for _, s := range snaps {
		row := map[string]any{
			"id":           s.ID,
			"target_id":    s.TargetID,
			"collected_at": s.CollectedAt,
			"pg_version":   s.PGVersion,
			"payload":      json.RawMessage(s.Payload),
		}
		if err := enc.Encode(row); err != nil {
			return err
		}
	}

	return nil
}
