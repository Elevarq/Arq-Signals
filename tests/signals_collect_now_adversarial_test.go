package tests

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/elevarq/arq-signals/internal/safety"
)

// ---------------------------------------------------------------------------
// R082/R083 stabilization — adversarial input + concurrency
// ---------------------------------------------------------------------------

// TestCollectNowMalformedJSONWithPartialValidFields verifies that a
// body which mixes valid-looking fields with at-least-one type error
// is rejected cleanly with `collect_now_rejected` /
// `error=invalid_json` and a 400 — never silently coerced into a
// partial cycle. The combinations cover (a) a number where a string
// is expected, (b) an object where an array is expected, (c) a
// trailing-comma syntax error.
// Traces: ARQ-SIGNALS-R082 / stabilization
func TestCollectNowMalformedJSONWithPartialValidFields(t *testing.T) {
	handler, cleanup := makeTargetTestHandler(t, twoTargets())
	defer cleanup()

	cases := []struct {
		name string
		body string
	}{
		{"request_id_is_number", `{"request_id": 123, "targets": ["primary"]}`},
		{"targets_is_object", `{"targets": {"primary": true}}`},
		{"trailing_comma", `{"request_id":"abc","targets":["primary"],}`},
		{"truncated_object", `{"request_id":"abc",`},
		{"top_level_array", `["primary","standby"]`},
		{"top_level_string", `"hi"`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := captureAuditLogs(t, func() {
				req := httptest.NewRequest("POST", "/collect/now", bytes.NewReader([]byte(c.body)))
				req.Header.Set("Authorization", "Bearer "+testAPIToken)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)
				if w.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400 (body=%q)", w.Code, c.body)
				}
			})
			if !strings.Contains(out, "audit_event=collect_now_rejected") {
				t.Errorf("missing collect_now_rejected event:\n%s", out)
			}
			if !strings.Contains(out, "error=invalid_json") {
				t.Errorf("expected error=invalid_json, got:\n%s", out)
			}
		})
	}
}

// TestCollectNowJSONNullAndEmpty verifies that JSON `null` and `{}`
// degrade to "no fields supplied" — same outcome as an empty body —
// rather than tripping a validation error. Belt-and-suspenders for
// callers that send a deliberately empty payload with the
// Content-Type header set.
// Traces: ARQ-SIGNALS-R082
func TestCollectNowJSONNullAndEmpty(t *testing.T) {
	handler, cleanup := makeTargetTestHandler(t, twoTargets())
	defer cleanup()

	for _, body := range []string{"null", "{}"} {
		req := httptest.NewRequest("POST", "/collect/now", bytes.NewReader([]byte(body)))
		req.Header.Set("Authorization", "Bearer "+testAPIToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusAccepted {
			t.Errorf("body %q: status = %d, want 202", body, w.Code)
		}
	}
}

// TestCollectNowLargeBody verifies the daemon doesn't crash, OOM, or
// panic on a body sized in the hundreds of KB. The current contract
// is "no explicit MaxBytesReader limit" — this test fixes the
// existing behaviour (clean handling, returns within time, body is
// either parsed or rejected as invalid_json). If a future
// MaxBytesReader is added, this test will need updating to assert
// 413 / 400 with a payload-too-large category.
// Traces: ARQ-SIGNALS-R082 / stabilization
func TestCollectNowLargeBody(t *testing.T) {
	handler, cleanup := makeTargetTestHandler(t, twoTargets())
	defer cleanup()

	// 256 KB JSON body: a giant `request_id` value (well over the
	// 32-char regex limit) plus valid targets. Exercises the parse
	// path on a large input.
	huge := strings.Repeat("a", 256*1024)
	body := `{"request_id":"` + huge + `","targets":["primary"]}`

	req := httptest.NewRequest("POST", "/collect/now", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+testAPIToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// The huge request_id violates `^[A-Za-z0-9_-]{1,32}$` so we
	// expect a clean 400 with error=invalid_request_id, NOT a panic
	// or OOM.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "request_id") {
		t.Errorf("response should mention request_id rejection, got: %s", w.Body.String())
	}
}

// TestCollectNowConcurrentSpam fires N parallel /collect/now POSTs
// against a single handler under -race. The buffer-1 channel + the
// running.TryLock guard mean at most one cycle runs at a time;
// every other request must receive 202 (queued or queue-full
// followed by collect_now_dropped). The test asserts:
//
//   - No panic, no goroutine leak, no -race detection.
//   - Total responses == N.
//   - Each is 202 Accepted (the auth and validation paths are
//     uniform across requests).
//   - The audit log contains at least one collect_now_dropped event,
//     proving the buffer-full path was exercised under contention.
//
// Traces: ARQ-SIGNALS-R082 / R032 / stabilization
func TestCollectNowConcurrentSpam(t *testing.T) {
	handler, cleanup := makeTargetTestHandler(t, twoTargets())
	defer cleanup()

	const n = 64
	var ok202 atomic.Int64
	var other atomic.Int64

	out := captureAuditLogs(t, func() {
		var wg sync.WaitGroup
		wg.Add(n)
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				<-start
				req := httptest.NewRequest("POST", "/collect/now", nil)
				req.Header.Set("Authorization", "Bearer "+testAPIToken)
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)
				if w.Code == http.StatusAccepted {
					ok202.Add(1)
				} else {
					other.Add(1)
				}
			}()
		}
		close(start) // release all goroutines simultaneously
		wg.Wait()
	})

	if got := ok202.Load(); got != int64(n) {
		t.Errorf("expected %d 202 responses, got %d (other=%d)", n, got, other.Load())
	}

	// At least one drop should be visible in the audit stream — the
	// channel buffer is 1, so 64 simultaneous requests cannot all
	// queue.
	if !strings.Contains(out, "audit_event=collect_now_dropped") {
		t.Errorf("expected at least one collect_now_dropped under contention; audit output had none:\n--- last 4KB of audit ---\n%s",
			lastN(out, 4096))
	}
}

// lastN returns the last n characters of s for terse error output.
func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// ---------------------------------------------------------------------------
// F1 fix: audit-attribute allowlist for the configuration-status boolean
// ---------------------------------------------------------------------------

// TestModeConfiguredEventCarriesAllowlistedKey verifies the R078
// audit-attribute denylist no longer eats the
// `arq_control_plane_token_configured` boolean. Before the
// stabilization patch the substring match on "token" filtered the
// key out, so the spec-promised attribute never reached the audit
// stream. The fix added the key to a small, hand-curated allow list.
// Traces: ARQ-SIGNALS-R078 / R083 / stabilization
func TestModeConfiguredEventCarriesAllowlistedKey(t *testing.T) {
	out := captureAuditLogs(t, func() {
		safety.AuditLog("mode_configured",
			"mode", "arq_managed",
			"arq_control_plane_token_configured", true,
		)
	})

	if !strings.Contains(out, "audit_event=mode_configured") {
		t.Fatalf("missing mode_configured event:\n%s", out)
	}
	if !strings.Contains(out, "arq_control_plane_token_configured=true") {
		t.Errorf("expected arq_control_plane_token_configured=true to survive the audit denylist; got:\n%s", out)
	}
}
