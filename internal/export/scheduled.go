// Copyright (c) 2026 Scantr LLC. All rights reserved.
// Elevarq is a trade name of Scantr LLC.
// This file is part of Elevarq Signals. Use is governed by the
// commercial license at LICENSE in the repository root.

// Scheduled auto-export (#350). Signals collects and stores on a schedule;
// this writes the latest snapshot out to a configured file location on each
// collection cycle, so the destination always holds a fresh export with no
// per-cycle operator action. What (if anything) consumes those files is out
// of scope — Signals has no knowledge of any downstream consumer.
package export

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// snapshotSource is the narrow slice of *Builder the scheduled exporter
// needs: writing a scoped export to a writer, and enumerating the targets
// that have a stored snapshot. *Builder satisfies it; tests substitute a
// fake so ExportLatest is exercised without a database.
type snapshotSource interface {
	WriteTo(w io.Writer, opts Options) error
	LatestTargetIDs() ([]int64, error)
}

// ScheduledExporter writes the latest per-target snapshot export ZIPs to a
// directory — one file per database per invocation — using the shared
// long-lived Builder. It is safe to call from the collector's post-cycle
// hook.
type ScheduledExporter struct {
	builder    snapshotSource
	dest       string
	instanceID string
	now        func() time.Time
	logf       func(msg string, args ...any)
	// Retention bounds for the export directory (#385). Zero = unbounded
	// (the pre-#385 behaviour). Set via SetRetention.
	retentionDays int
	maxFiles      int
}

// NewScheduledExporter constructs the exporter. `dest` is the destination
// directory; `instanceID` disambiguates files when several Signals
// instances write to one shared directory (#350). `now`/`logf` default to
// wall-clock / no-op when nil.
func NewScheduledExporter(b snapshotSource, dest, instanceID string, now func() time.Time, logf func(string, ...any)) *ScheduledExporter {
	if now == nil {
		now = time.Now
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &ScheduledExporter{builder: b, dest: dest, instanceID: instanceID, now: now, logf: logf}
}

// SetRetention bounds the export directory (#385): after each cycle the
// exporter prunes its OWN older ZIPs per target — keeping at most maxFiles
// (when > 0) and deleting any older than retentionDays (when > 0). Zero for
// both is unbounded (the pre-#385 behaviour). Only this instance's files
// (`<instance>-t<target>-*.zip`) are pruned, so several instances sharing one
// directory never delete each other's exports.
func (e *ScheduledExporter) SetRetention(retentionDays, maxFiles int) {
	e.retentionDays = retentionDays
	e.maxFiles = maxFiles
}

// pruneTarget deletes this instance's older export ZIPs for one target so the
// scheduled-export directory stays bounded (#385). It keeps the newest
// maxFiles and deletes any older than retentionDays; both bounds apply when
// set. Best-effort — a failed unlink is logged and never fails the export
// (the fresh ZIP for this cycle is already durably written). The per-target
// timestamped filename sorts lexicographically == chronologically, so the tail
// of the sorted list is the newest.
func (e *ScheduledExporter) pruneTarget(targetID int64) {
	if e.retentionDays <= 0 && e.maxFiles <= 0 {
		return // unbounded
	}
	inst := instanceToken.ReplaceAllString(strings.TrimSpace(e.instanceID), "_")
	if inst == "" {
		inst = "signals"
	}
	matches, err := filepath.Glob(filepath.Join(e.dest, fmt.Sprintf("%s-t%d-*.zip", inst, targetID)))
	if err != nil || len(matches) == 0 {
		return
	}
	sort.Strings(matches) // oldest first (timestamp in the name)

	remove := make(map[string]bool)
	if e.maxFiles > 0 && len(matches) > e.maxFiles {
		for _, p := range matches[:len(matches)-e.maxFiles] {
			remove[p] = true
		}
	}
	if e.retentionDays > 0 {
		cutoff := e.now().Add(-time.Duration(e.retentionDays) * 24 * time.Hour)
		for _, p := range matches {
			if info, serr := os.Stat(p); serr == nil && info.ModTime().Before(cutoff) {
				remove[p] = true
			}
		}
	}
	for p := range remove {
		if rerr := os.Remove(p); rerr != nil {
			e.logf("scheduled export: prune %s failed (%v); leaving it in place", filepath.Base(p), rerr)
		}
	}
}

// instanceToken keeps the filename component filesystem-safe and flat.
var instanceToken = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// exportFilename builds a FLAT, per-instance, per-target, per-timestamp
// filename: `<instance>-t<targetID>-<RFC3339Nano-ish UTC>.zip`. Flat (no
// subdirectories) so a downstream flat directory watcher sees it; instance
// + target + nanosecond timestamp make it unique across instances, targets,
// and cycles, so a new export never overwrites a prior one and the several
// targets written in one cycle never collide.
func (e *ScheduledExporter) exportFilename(targetID int64) string {
	inst := instanceToken.ReplaceAllString(strings.TrimSpace(e.instanceID), "_")
	if inst == "" {
		inst = "signals"
	}
	ts := e.now().UTC().Format("20060102T150405.000000000Z")
	return fmt.Sprintf("%s-t%d-%s.zip", inst, targetID, ts)
}

// ExportLatest writes the latest snapshot for EACH target as its own export
// ZIP into the destination directory (#350) — one file per database. A
// combined multi-target export is read by a downstream consumer (the
// Analyzer inbox sweeper) as a single database, so each target is exported
// separately with Options{TargetID}. Every write is atomic (temp-file +
// rename), so a consumer watching the directory never observes a
// partially-written ZIP. Returns the paths written. On a per-target failure
// it returns the files already written plus the error, leaving no partial
// final file; the caller (the collector hook) logs and continues — a failed
// export must never disrupt collection.
func (e *ScheduledExporter) ExportLatest(_ context.Context) ([]string, error) {
	if e.dest == "" {
		return nil, fmt.Errorf("scheduled export: destination not configured")
	}
	if err := os.MkdirAll(e.dest, 0o755); err != nil {
		return nil, fmt.Errorf("scheduled export: mkdir %s: %w", e.dest, err)
	}
	ids, err := e.builder.LatestTargetIDs()
	if err != nil {
		return nil, fmt.Errorf("scheduled export: list targets: %w", err)
	}
	written := make([]string, 0, len(ids))
	for _, id := range ids {
		final, err := e.exportOne(id)
		if err != nil {
			return written, fmt.Errorf("scheduled export: target %d: %w", id, err)
		}
		written = append(written, final)
		e.pruneTarget(id) // #385 — bound the dir; best-effort, never fails the export
	}
	return written, nil
}

// exportOne writes one target's latest-snapshot export ZIP atomically
// (temp-file + rename) and returns the final path. It leaves no partial
// final file on error.
func (e *ScheduledExporter) exportOne(targetID int64) (string, error) {
	name := e.exportFilename(targetID)
	final := filepath.Join(e.dest, name)
	tmp := filepath.Join(e.dest, "."+name+".tmp")

	f, err := os.Create(tmp)
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}
	werr := e.builder.WriteTo(f, Options{TargetID: targetID})
	cerr := f.Close()
	if werr != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("write: %w", werr)
	}
	if cerr != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("close: %w", cerr)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("rename: %w", err)
	}
	return final, nil
}
