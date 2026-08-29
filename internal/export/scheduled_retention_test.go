// Copyright (c) 2026 Scantr LLC. All rights reserved.
// Elevarq is a trade name of Scantr LLC.
// This file is part of Elevarq Signals. Use is governed by the
// commercial license at LICENSE in the repository root.

package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func zipsFor(t *testing.T, dest, inst string, targetID int64) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dest, inst+"-t"+itoa(targetID)+"-*.zip"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	return m
}

func itoa(n int64) string {
	// small helper so the test does not import strconv just for this.
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// #385 — ExportMaxFiles keeps only the newest N ZIPs per target; each cycle
// prunes this instance's older exports.
func TestScheduledExporter_Retention_MaxFiles(t *testing.T) {
	dest := t.TempDir()
	fw := &fakeWriter{targetIDs: []int64{1}, payload: []byte("PK\x03\x04zip")}
	base := time.Date(2026, 8, 14, 9, 30, 0, 0, time.UTC)
	clk := base
	se := NewScheduledExporter(fw, dest, "signals-prod-1", func() time.Time { return clk }, nil)
	se.SetRetention(0, 2) // keep newest 2

	for i := 0; i < 5; i++ {
		clk = base.Add(time.Duration(i) * time.Minute) // distinct timestamp per cycle
		if _, err := se.ExportLatest(context.Background()); err != nil {
			t.Fatalf("cycle %d: ExportLatest: %v", i, err)
		}
	}

	got := zipsFor(t, dest, "signals-prod-1", 1)
	if len(got) != 2 {
		t.Fatalf("after 5 cycles with max_files=2, want 2 ZIPs, got %d: %v", len(got), got)
	}
	// The two survivors must be the newest two timestamps (cycles 3 and 4).
	for _, want := range []string{
		"signals-prod-1-t1-20260814T093300.000000000Z.zip",
		"signals-prod-1-t1-20260814T093400.000000000Z.zip",
	} {
		if _, err := os.Stat(filepath.Join(dest, want)); err != nil {
			t.Errorf("newest survivor %q missing: %v", want, err)
		}
	}
}

// #385 — ExportRetentionDays deletes ZIPs older than the cutoff by mtime.
func TestScheduledExporter_Retention_Days(t *testing.T) {
	dest := t.TempDir()
	now := time.Date(2026, 8, 14, 9, 30, 0, 0, time.UTC)
	se := NewScheduledExporter(
		&fakeWriter{targetIDs: []int64{7}},
		dest, "signals-prod-1", func() time.Time { return now }, nil,
	)
	se.SetRetention(2, 0) // 2-day age bound

	mk := func(name string, age time.Duration) {
		p := filepath.Join(dest, name)
		if err := os.WriteFile(p, []byte("z"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		mt := now.Add(-age)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
	}
	mk("signals-prod-1-t7-old1.zip", 5*24*time.Hour)  // 5 days → pruned
	mk("signals-prod-1-t7-old2.zip", 3*24*time.Hour)  // 3 days → pruned
	mk("signals-prod-1-t7-fresh.zip", 1*24*time.Hour) // 1 day → kept
	// Another instance's file for the same target must NOT be touched.
	mk("other-inst-t7-old.zip", 9*24*time.Hour)

	se.pruneTarget(7)

	if got := zipsFor(t, dest, "signals-prod-1", 7); len(got) != 1 {
		t.Errorf("retention_days=2: want 1 fresh ZIP left, got %d: %v", len(got), got)
	}
	if _, err := os.Stat(filepath.Join(dest, "other-inst-t7-old.zip")); err != nil {
		t.Errorf("another instance's file was pruned (must not be): %v", err)
	}
}

// #385 — the default (0/0) is unbounded: nothing is pruned (pre-#385 behaviour).
func TestScheduledExporter_Retention_UnboundedByDefault(t *testing.T) {
	dest := t.TempDir()
	base := time.Date(2026, 8, 14, 9, 30, 0, 0, time.UTC)
	clk := base
	se := NewScheduledExporter(
		&fakeWriter{targetIDs: []int64{1}, payload: []byte("z")},
		dest, "signals-prod-1", func() time.Time { return clk }, nil,
	)
	// No SetRetention call → 0/0.
	for i := 0; i < 4; i++ {
		clk = base.Add(time.Duration(i) * time.Minute)
		if _, err := se.ExportLatest(context.Background()); err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
	}
	if got := zipsFor(t, dest, "signals-prod-1", 1); len(got) != 4 {
		t.Errorf("unbounded default: want all 4 ZIPs, got %d", len(got))
	}
}
