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

// Builder creates a ZIP export of collected data.
type Builder struct {
	store             *db.DB
	instanceID        string
	unsafeMode        bool
	unsafeReasonsFunc func() []string
	collectorStatus   *collector.CollectorStatusFile
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

// WriteTo writes the ZIP export to the given writer.
func (b *Builder) WriteTo(w io.Writer, opts Options) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	if err := b.writeMetadata(zw); err != nil {
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

func (b *Builder) writeMetadata(zw *zip.Writer) error {
	f, err := zw.Create("metadata.json")
	if err != nil {
		return err
	}

	data := map[string]any{
		"schema_version":    snapshot.SchemaVersion,
		"instance_id":       b.instanceID,
		"collector_version": safety.Version,
		"collector_commit":  safety.Commit,
		"collected_at":      time.Now().UTC().Format(time.RFC3339),
		"unsafe_mode":       b.unsafeMode,
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
			SchemaVersion: "1",
			TargetName:    targetName,
			CollectedAt:   time.Now().UTC().Format(time.RFC3339),
			Collectors:    statuses,
		}
		file.Sort()
		return json.NewEncoder(f).Encode(file)
	}

	// Instance-level: use provided status or empty
	if b.collectorStatus != nil {
		b.collectorStatus.Sort()
		return json.NewEncoder(f).Encode(b.collectorStatus)
	}

	empty := collector.CollectorStatusFile{
		SchemaVersion: "1",
		CollectedAt:   time.Now().UTC().Format(time.RFC3339),
		Collectors:    []collector.CollectorStatus{},
	}
	return json.NewEncoder(f).Encode(empty)
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
		if r.Error != "" {
			continue
		}
		res, err := b.store.GetQueryResultByRunID(r.ID)
		if err != nil || res == nil {
			continue
		}
		decoded, err := db.DecodeNDJSON(res.Payload, res.Compressed)
		if err != nil {
			continue
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
