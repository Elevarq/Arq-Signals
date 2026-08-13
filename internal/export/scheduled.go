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
	"strings"
	"time"
)

// snapshotWriter is the narrow slice of *Builder the scheduled exporter
// needs — writing a scoped export to a writer. *Builder satisfies it; tests
// substitute a fake so ExportLatest is exercised without a database.
type snapshotWriter interface {
	WriteTo(w io.Writer, opts Options) error
}

// ScheduledExporter writes the latest-snapshot export ZIP to a directory,
// one file per invocation, using the shared long-lived Builder. It is
// safe to call from the collector's post-cycle hook.
type ScheduledExporter struct {
	builder    snapshotWriter
	dest       string
	instanceID string
	now        func() time.Time
	logf       func(msg string, args ...any)
}

// NewScheduledExporter constructs the exporter. `dest` is the destination
// directory; `instanceID` disambiguates files when several Signals
// instances write to one shared directory (#350). `now`/`logf` default to
// wall-clock / no-op when nil.
func NewScheduledExporter(b snapshotWriter, dest, instanceID string, now func() time.Time, logf func(string, ...any)) *ScheduledExporter {
	if now == nil {
		now = time.Now
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &ScheduledExporter{builder: b, dest: dest, instanceID: instanceID, now: now, logf: logf}
}

// instanceToken keeps the filename component filesystem-safe and flat.
var instanceToken = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// exportFilename builds a FLAT, per-instance, per-timestamp filename:
// `<instance>-<RFC3339Nano-ish UTC>.zip`. Flat (no subdirectories) so a
// downstream flat directory watcher sees it; instance + nanosecond
// timestamp make it unique across instances and cycles, so a new export
// never overwrites a prior one.
func (e *ScheduledExporter) exportFilename() string {
	inst := instanceToken.ReplaceAllString(strings.TrimSpace(e.instanceID), "_")
	if inst == "" {
		inst = "signals"
	}
	ts := e.now().UTC().Format("20060102T150405.000000000Z")
	return fmt.Sprintf("%s-%s.zip", inst, ts)
}

// ExportLatest builds the latest-snapshot export (default scope) and writes
// it atomically into the destination directory (#350). The write is
// temp-file + rename, so a consumer watching the directory never observes a
// partially-written ZIP. Returns an error without leaving a partial final
// file; a caller (the collector hook) logs and continues — a failed export
// must never disrupt collection.
func (e *ScheduledExporter) ExportLatest(_ context.Context) (string, error) {
	if e.dest == "" {
		return "", fmt.Errorf("scheduled export: destination not configured")
	}
	if err := os.MkdirAll(e.dest, 0o755); err != nil {
		return "", fmt.Errorf("scheduled export: mkdir %s: %w", e.dest, err)
	}
	name := e.exportFilename()
	final := filepath.Join(e.dest, name)
	tmp := filepath.Join(e.dest, "."+name+".tmp")

	f, err := os.Create(tmp)
	if err != nil {
		return "", fmt.Errorf("scheduled export: create temp: %w", err)
	}
	// Default Options = the latest snapshot per target (the same scope the
	// on-demand GET /export uses with no filter).
	werr := e.builder.WriteTo(f, Options{})
	cerr := f.Close()
	if werr != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("scheduled export: write: %w", werr)
	}
	if cerr != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("scheduled export: close: %w", cerr)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("scheduled export: rename: %w", err)
	}
	return final, nil
}
