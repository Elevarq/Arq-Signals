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
type fakeWriter struct {
	payload []byte
	err     error
}

func (f *fakeWriter) WriteTo(w io.Writer, _ Options) error {
	if f.err != nil {
		return f.err
	}
	_, err := w.Write(f.payload)
	return err
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestScheduledExporter_ExportLatest_WritesAtomically(t *testing.T) {
	dest := t.TempDir()
	now := time.Date(2026, 8, 13, 9, 30, 0, 0, time.UTC)
	se := NewScheduledExporter(&fakeWriter{payload: []byte("PK\x03\x04zip-bytes")}, dest, "signals-prod-1", fixedClock(now), nil)

	path, err := se.ExportLatest(context.Background())
	if err != nil {
		t.Fatalf("ExportLatest: %v", err)
	}
	// Flat file directly in dest, instance-prefixed, .zip.
	if filepath.Dir(path) != dest {
		t.Errorf("wrote to %q, want flat in %q (no subdirectory)", path, dest)
	}
	base := filepath.Base(path)
	if !strings.HasPrefix(base, "signals-prod-1-") || !strings.HasSuffix(base, ".zip") {
		t.Errorf("filename %q, want signals-prod-1-<ts>.zip", base)
	}
	// Contents are the builder's bytes; no leftover temp file.
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "PK\x03\x04zip-bytes" {
		t.Errorf("file contents = %q (err %v), want the export payload", got, err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 1 {
		t.Errorf("dest has %d entries, want exactly 1 (no leftover .tmp)", len(entries))
	}
}

func TestScheduledExporter_ExportLatest_WriteFailureLeavesNoFile(t *testing.T) {
	dest := t.TempDir()
	se := NewScheduledExporter(&fakeWriter{err: errors.New("boom")}, dest, "inst", fixedClock(time.Now()), nil)

	if _, err := se.ExportLatest(context.Background()); err == nil {
		t.Fatal("expected error from a failing writer")
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
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
		{"signals-prod-1", "signals-prod-1-"},
		{"weird id/with:bad*chars", "weird_id_with_bad_chars-"}, // sanitized, flat
		{"  ", "signals-"}, // blank falls back
		{"", "signals-"},
	}
	for _, tc := range cases {
		se := NewScheduledExporter(&fakeWriter{}, "/x", tc.instance, fixedClock(now), nil)
		name := se.exportFilename()
		if !strings.HasPrefix(name, tc.wantPrefix) {
			t.Errorf("instance %q -> %q, want prefix %q", tc.instance, name, tc.wantPrefix)
		}
		if !strings.HasSuffix(name, ".zip") || strings.ContainsAny(name, "/\\") {
			t.Errorf("filename %q must be a flat *.zip", name)
		}
	}
	// Distinct timestamps yield distinct names (never overwrite a prior export).
	seA := NewScheduledExporter(&fakeWriter{}, "/x", "i", fixedClock(now), nil)
	seB := NewScheduledExporter(&fakeWriter{}, "/x", "i", fixedClock(now.Add(time.Second)), nil)
	if seA.exportFilename() == seB.exportFilename() {
		t.Error("different times produced the same filename")
	}
}
