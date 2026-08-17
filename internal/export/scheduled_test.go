// Copyright (c) 2026 Scantr LLC. All rights reserved.
// Elevarq is a trade name of Scantr LLC.
// This file is part of Elevarq Signals. Use is governed by the
// commercial license at LICENSE in the repository root.

package export

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeWriter stands in for *Builder so ExportLatest is testable without a DB.
// targetIDs is what LatestTargetIDs reports; payloadFor lets a test vary the
// bytes per target (defaults to `payload` when nil).
type fakeWriter struct {
	targetIDs  []int64
	targetsErr error
	payload    []byte
	payloadFor func(targetID int64) []byte
	err        error
	failFor    map[int64]bool // targets whose WriteTo should fail
}

func (f *fakeWriter) LatestTargetIDs() ([]int64, error) {
	return f.targetIDs, f.targetsErr
}

func (f *fakeWriter) WriteTo(w io.Writer, opts Options) error {
	if f.err != nil {
		return f.err
	}
	if f.failFor != nil && f.failFor[opts.TargetID] {
		// Write some bytes THEN fail, to prove a partial temp file is
		// neither promoted nor left behind.
		_, _ = w.Write([]byte("partial"))
		return errors.New("boom")
	}
	if f.payloadFor != nil {
		_, err := w.Write(f.payloadFor(opts.TargetID))
		return err
	}
	_, err := w.Write(f.payload)
	return err
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// TC-SIG-130 (normal): each target's latest snapshot is written as its own
// flat, instance+target-prefixed, atomically-written ZIP — one file per
// database, never a combined multi-target ZIP.
func TestScheduledExporter_ExportLatest_OneFilePerTarget(t *testing.T) {
	dest := t.TempDir()
	now := time.Date(2026, 8, 13, 9, 30, 0, 0, time.UTC)
	fw := &fakeWriter{
		targetIDs:  []int64{1, 2},
		payloadFor: func(id int64) []byte { return []byte{'t', byte('0' + id)} },
	}
	se := NewScheduledExporter(fw, dest, "signals-prod-1", fixedClock(now), nil)

	paths, err := se.ExportLatest(context.Background())
	if err != nil {
		t.Fatalf("ExportLatest: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("wrote %d files, want 2 (one per target): %v", len(paths), paths)
	}
	for _, p := range paths {
		if filepath.Dir(p) != dest {
			t.Errorf("wrote to %q, want flat in %q (no subdirectory)", p, dest)
		}
	}
	want1 := "signals-prod-1-t1-20260813T093000.000000000Z.zip"
	want2 := "signals-prod-1-t2-20260813T093000.000000000Z.zip"
	b1, err := os.ReadFile(filepath.Join(dest, want1))
	if err != nil || string(b1) != "t1" {
		t.Errorf("target 1 file %q = %q (err %v), want scoped payload %q", want1, b1, err, "t1")
	}
	b2, err := os.ReadFile(filepath.Join(dest, want2))
	if err != nil || string(b2) != "t2" {
		t.Errorf("target 2 file %q = %q (err %v), want scoped payload %q", want2, b2, err, "t2")
	}
	// Exactly the two ZIPs, no leftover temp file.
	entries, _ := os.ReadDir(dest)
	if len(entries) != 2 {
		t.Errorf("dest has %d entries, want exactly 2 (no leftover .tmp)", len(entries))
	}
}

// TC-SIG-131 (boundary): a single target still produces exactly one file,
// and the export is scoped to that target (Options.TargetID set, not the
// combined default).
func TestScheduledExporter_ExportLatest_SingleTargetIsScoped(t *testing.T) {
	dest := t.TempDir()
	var gotOpts []Options
	fw := &fakeWriter{targetIDs: []int64{7}, payload: []byte("PK\x03\x04zip")}
	// Wrap to capture the Options passed through.
	se := NewScheduledExporter(optsCapture{fw, &gotOpts}, dest, "inst", fixedClock(time.Now()), nil)

	paths, err := se.ExportLatest(context.Background())
	if err != nil {
		t.Fatalf("ExportLatest: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("want 1 file, got %d", len(paths))
	}
	if len(gotOpts) != 1 || gotOpts[0].TargetID != 7 {
		t.Errorf("export was not target-scoped: opts=%+v (want TargetID=7)", gotOpts)
	}
}

// TC-SIG-132 (invalid/source failure): when target enumeration fails, the
// export writes nothing and surfaces the error.
func TestScheduledExporter_ExportLatest_TargetListErrorWritesNothing(t *testing.T) {
	dest := t.TempDir()
	fw := &fakeWriter{targetsErr: errors.New("db unavailable")}
	se := NewScheduledExporter(fw, dest, "inst", fixedClock(time.Now()), nil)

	if _, err := se.ExportLatest(context.Background()); err == nil {
		t.Fatal("expected an error when listing targets fails")
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Errorf("dest has %d entries, want 0", len(entries))
	}
}

// TC-SIG-133 (failure): a per-target write failure leaves no partial/temp
// file for that target; the error is returned and the caller (collector
// hook) logs and continues.
func TestScheduledExporter_ExportLatest_WriteFailureLeavesNoFile(t *testing.T) {
	dest := t.TempDir()
	fw := &fakeWriter{targetIDs: []int64{1}, failFor: map[int64]bool{1: true}}
	se := NewScheduledExporter(fw, dest, "inst", fixedClock(time.Now()), nil)

	if _, err := se.ExportLatest(context.Background()); err == nil {
		t.Fatal("expected error from a failing writer")
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Errorf("dest has %d entries after a failed export, want 0 (no partial/temp file)", len(entries))
	}
}

func TestScheduledExporter_ExportLatest_NoDestErrors(t *testing.T) {
	se := NewScheduledExporter(&fakeWriter{}, "", "inst", nil, nil)
	if _, err := se.ExportLatest(context.Background()); err == nil {
		t.Error("expected an error when the destination is not configured")
	}
}

func TestScheduledExporter_exportFilename(t *testing.T) {
	now := time.Date(2026, 8, 13, 9, 30, 5, 123456789, time.UTC)
	cases := []struct {
		instance   string
		wantPrefix string
	}{
		{"signals-prod-1", "signals-prod-1-t3-"},
		{"weird id/with:bad*chars", "weird_id_with_bad_chars-t3-"}, // sanitized, flat
		{"  ", "signals-t3-"},                                      // blank falls back
		{"", "signals-t3-"},
	}
	for _, tc := range cases {
		se := NewScheduledExporter(&fakeWriter{}, "/x", tc.instance, fixedClock(now), nil)
		name := se.exportFilename(3)
		if !strings.HasPrefix(name, tc.wantPrefix) {
			t.Errorf("instance %q -> %q, want prefix %q", tc.instance, name, tc.wantPrefix)
		}
		if !strings.HasSuffix(name, ".zip") || strings.ContainsAny(name, "/\\") {
			t.Errorf("filename %q must be a flat *.zip", name)
		}
	}
	// Two targets in one cycle (same timestamp) never collide.
	se := NewScheduledExporter(&fakeWriter{}, "/x", "i", fixedClock(now), nil)
	if se.exportFilename(1) == se.exportFilename(2) {
		t.Error("different targets produced the same filename in one cycle")
	}
	// Distinct timestamps yield distinct names (never overwrite a prior export).
	seA := NewScheduledExporter(&fakeWriter{}, "/x", "i", fixedClock(now), nil)
	seB := NewScheduledExporter(&fakeWriter{}, "/x", "i", fixedClock(now.Add(time.Second)), nil)
	if seA.exportFilename(1) == seB.exportFilename(1) {
		t.Error("different times produced the same filename")
	}
}

// optsCapture wraps a snapshotSource to record the Options passed to WriteTo,
// so a test can assert the export was target-scoped.
type optsCapture struct {
	inner *fakeWriter
	got   *[]Options
}

func (o optsCapture) LatestTargetIDs() ([]int64, error) { return o.inner.LatestTargetIDs() }
func (o optsCapture) WriteTo(w io.Writer, opts Options) error {
	*o.got = append(*o.got, opts)
	return o.inner.WriteTo(w, opts)
}
