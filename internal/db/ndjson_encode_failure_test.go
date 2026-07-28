// Copyright (c) 2026 Scantr LLC. All rights reserved.
// Elevarq is a trade name of Scantr LLC.
// This file is part of Elevarq Signals. Use is governed by the
// commercial license at LICENSE in the repository root.

package db

import (
	"math"
	"testing"
)

// TestEncodeNDJSON_FailsOnUnmarshalableValue documents the failure mode
// the snapshot status/payload invariant (#312) must handle: when a row
// carries a value json cannot marshal (e.g. NaN), EncodeNDJSON returns
// an error. The collector must then record the run as FAILED, never as
// an orphaned success — otherwise the status manifest claims success
// with row_count rows while query_results carries no payload.
func TestEncodeNDJSON_FailsOnUnmarshalableValue(t *testing.T) {
	rows := []map[string]any{{"v": math.NaN()}}
	if _, _, _, err := EncodeNDJSON(rows); err == nil {
		t.Fatal("expected EncodeNDJSON to error on a NaN value, got nil — the #312 invariant relies on this error surfacing")
	}
}
