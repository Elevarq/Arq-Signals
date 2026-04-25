package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/elevarq/arq-signals/internal/collector"
	"github.com/elevarq/arq-signals/internal/config"
	"github.com/elevarq/arq-signals/internal/db"
	"github.com/elevarq/arq-signals/internal/export"
	"github.com/elevarq/arq-signals/internal/metrics"
	"github.com/elevarq/arq-signals/internal/safety"
)

// Deps holds handler dependencies.
type Deps struct {
	DB        *db.DB
	Collector *collector.Collector
	Exporter  *export.Builder
	Targets   []config.TargetConfig
	// Metrics is the optional Prometheus registry. When nil the
	// /metrics endpoint is not registered. Pass non-nil only when
	// signals.metrics_enabled is true.
	Metrics *metrics.Registry
	// MetricsPath is the URL path the /metrics endpoint is mounted on.
	// Ignored when Metrics is nil.
	MetricsPath string
}

// Server is the Arq Signals HTTP API server.
type Server struct {
	httpServer *http.Server
	deps       *Deps
}

// NewServer creates a new API Server with signals-only endpoints.
func NewServer(addr string, readTimeout, writeTimeout time.Duration, apiToken string, deps *Deps) *Server {
	mux := http.NewServeMux()

	// Register signals-only routes.
	mux.HandleFunc("GET /health", handleHealth(deps))
	mux.HandleFunc("GET /status", handleStatus(deps))
	mux.HandleFunc("POST /collect/now", handleCollectNow(deps))
	mux.HandleFunc("GET /export", handleExport(deps))

	// Optional Prometheus /metrics endpoint (R079). Off unless an
	// explicit Metrics registry is supplied. Inherits the same bearer
	// token auth as the rest of the API; operators that want
	// unauthenticated scraping should bind locally and use
	// network-level controls.
	if deps.Metrics != nil && deps.MetricsPath != "" {
		mux.Handle("GET "+deps.MetricsPath, promhttp.HandlerFor(
			deps.Metrics.Gatherer(),
			promhttp.HandlerOpts{
				ErrorLog:      slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
				ErrorHandling: promhttp.ContinueOnError,
			},
		))
	}

	// Wrap with middleware: recovery -> logging -> token auth.
	tokenLimiter := newTokenRateLimiter()
	handler := recoveryMiddleware(loggingMiddleware(tokenAuthMiddleware(apiToken, tokenLimiter)(mux)))

	return &Server{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
		},
		deps: deps,
	}
}

// Handler returns the HTTP handler for testing.
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// Start begins listening and serving. It blocks until the server stops.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	slog.Info("API server listening", "addr", s.httpServer.Addr)
	return s.httpServer.Serve(ln)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("API server shutting down")
	return s.httpServer.Shutdown(ctx)
}

// --- Handlers ---

func handleHealth(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": safety.Version,
		})
	}
}

func handleStatus(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapCount, _ := deps.DB.CountSnapshots()
		targets, _ := deps.DB.GetTargets()
		instanceID, _ := deps.DB.GetMeta("instance_id")

		var targetInfo []map[string]any
		for _, t := range targets {
			tInfo := map[string]any{
				"id":      t.ID,
				"name":    t.Name,
				"host":    t.Host,
				"port":    t.Port,
				"dbname":  t.DBName,
				"user":    t.Username,
				"sslmode": t.SSLMode,
				"enabled": t.Enabled,
			}
			// secret_type and secret_ref are intentionally omitted from
			// /status to avoid revealing credential source details.

			if lc := deps.DB.GetTargetLastCollected(t.ID); lc != "" {
				tInfo["last_collected"] = lc
			}

			targetInfo = append(targetInfo, tInfo)
		}

		queryCatalog, _ := deps.DB.GetQueryCatalog()

		resp := map[string]any{
			"instance_id":         instanceID,
			"version":             safety.Version,
			"target_count":        len(targets),
			"targets":             targetInfo,
			"snapshot_count":      snapCount,
			"query_catalog_count": len(queryCatalog),
			"last_collected":      deps.Collector.LastCollected(),
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// requestIDPattern bounds R082 Phase 2's `request_id` correlation
// identifier. ASCII alphanumerics, underscore, dash; up to 32 chars.
// Restricting the charset keeps audit logs greppable and prevents
// log-injection via control bytes. ULIDs satisfy this regex.
var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// reasonPattern bounds R082 Phase 2's `reason` label. Shares the
// request_id charset so neither field can carry log-injection
// payloads or unbounded whitespace. Up to 64 chars.
var reasonPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// handleCollectNow handles POST /collect/now with the full R082
// Phase 1 + Phase 2 contract.
//
// Body is optional. The historical empty-body path keeps collecting
// every enabled target and is unchanged at the HTTP level. Phase 2
// adds a single new behaviour: every request — empty body included —
// now emits an `audit_event=collect_now_requested` slog record with
// actor, requested_targets, accepted_targets, request_id, and (when
// supplied) reason. R078's audit-attribute denylist remains in
// force; secrets and SQL payloads never reach the audit log.
//
// Body shape (all fields optional):
//
//	{
//	  "targets": ["a", "b"],
//	  "request_id": "01J5K…",
//	  "reason": "scheduled_arq_cycle"
//	}
//
// Validation:
//   - targets empty array, unknown name, or disabled target → 400
//     with `accepted_targets` + per-name `rejected_targets`. Emits
//     `collect_now_rejected` audit event. (Phase 1 contract,
//     unchanged.)
//   - request_id present but doesn't match `^[A-Za-z0-9_-]{1,32}$`
//     → 400, emits `collect_now_rejected`.
//   - reason present but doesn't match `^[A-Za-z0-9_-]{1,64}$`
//     → 400, emits `collect_now_rejected`.
//   - Invalid JSON → 400, emits `collect_now_rejected`.
//
// When the channel buffer is full because a previous on-demand cycle
// is still queued, the request returns 202 (R032: no overlapping
// cycles, the in-flight filter wins) and emits a
// `collect_now_dropped` audit event so the request_id stays in the
// trail even when the cycle never fires.
//
// actor is always `local_operator` in Phase 2. The
// `arq_control_plane` actor value is reserved for Phase 3.
func handleCollectNow(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		body = bytes.TrimSpace(body)

		var allEnabled []string
		for _, t := range deps.Targets {
			if t.Enabled {
				allEnabled = append(allEnabled, t.Name)
			}
		}

		var (
			targetFilter   []string
			requestID      string
			reason         string
			suppliedReqID  bool
			suppliedReason bool
		)

		// Parse body when non-empty. Empty body retains Phase 1 backward
		// compatibility — no targets / no request_id / no reason.
		if len(body) > 0 {
			var req struct {
				Targets   *[]string `json:"targets,omitempty"`
				RequestID *string   `json:"request_id,omitempty"`
				Reason    *string   `json:"reason,omitempty"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				safety.AuditLog("collect_now_rejected",
					"actor", "local_operator",
					"error", "invalid_json",
				)
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "invalid JSON body",
				})
				return
			}

			// Validate request_id format before anything else so we can
			// surface it on subsequent rejection events.
			if req.RequestID != nil {
				if !requestIDPattern.MatchString(*req.RequestID) {
					safety.AuditLog("collect_now_rejected",
						"actor", "local_operator",
						"error", "invalid_request_id",
					)
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"error": "request_id must match ^[A-Za-z0-9_-]{1,32}$",
					})
					return
				}
				requestID = *req.RequestID
				suppliedReqID = true
			}

			if req.Reason != nil {
				if !reasonPattern.MatchString(*req.Reason) {
					safety.AuditLog("collect_now_rejected",
						"actor", "local_operator",
						"request_id", requestID,
						"error", "invalid_reason",
					)
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"error": "reason must match ^[A-Za-z0-9_-]{1,64}$",
					})
					return
				}
				reason = *req.Reason
				suppliedReason = true
			}

			if req.Targets != nil {
				if len(*req.Targets) == 0 {
					safety.AuditLog("collect_now_rejected",
						"actor", "local_operator",
						"request_id", requestID,
						"error", "empty_targets_array",
					)
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"error": "targets must be a non-empty array; omit the field to collect all enabled targets",
					})
					return
				}

				configured := make(map[string]config.TargetConfig, len(deps.Targets))
				for _, t := range deps.Targets {
					configured[t.Name] = t
				}

				type rejection struct {
					Name   string `json:"name"`
					Reason string `json:"reason"`
				}
				seen := make(map[string]bool, len(*req.Targets))
				accepted := make([]string, 0, len(*req.Targets))
				var rejected []rejection

				for _, name := range *req.Targets {
					if seen[name] {
						continue
					}
					seen[name] = true

					cfg, ok := configured[name]
					if !ok {
						rejected = append(rejected, rejection{Name: name, Reason: "unknown_target"})
						continue
					}
					if !cfg.Enabled {
						rejected = append(rejected, rejection{Name: name, Reason: "disabled_target"})
						continue
					}
					accepted = append(accepted, name)
				}

				if len(rejected) > 0 {
					rejectedAttrs := []any{
						"actor", "local_operator",
						"requested_targets", *req.Targets,
						"accepted_targets", accepted,
						"rejected_targets", rejected,
					}
					if suppliedReqID {
						rejectedAttrs = append(rejectedAttrs, "request_id", requestID)
					}
					if suppliedReason {
						rejectedAttrs = append(rejectedAttrs, "reason", reason)
					}
					safety.AuditLog("collect_now_rejected", rejectedAttrs...)
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"error":            "one or more targets cannot be collected",
						"accepted_targets": accepted,
						"rejected_targets": rejected,
					})
					return
				}

				targetFilter = accepted
			}
		}

		// Generate a ULID when the caller didn't supply a request_id
		// so cycle audit events always carry a correlation id.
		if requestID == "" {
			requestID = newRequestID()
		}

		// Compute the effective response/audit target set.
		responseTargets := targetFilter
		if responseTargets == nil {
			responseTargets = allEnabled
		}

		// Audit: every successful request emits collect_now_requested
		// before queuing. Phase 2 actor invariant: always
		// local_operator until Phase 3 introduces a separate
		// arq_control_plane token (R082).
		requestedAttrs := []any{
			"actor", "local_operator",
			"request_id", requestID,
			"requested_targets", requestedTargetsAuditValue(targetFilter, allEnabled),
			"accepted_targets", responseTargets,
		}
		if suppliedReason {
			requestedAttrs = append(requestedAttrs, "reason", reason)
		}
		safety.AuditLog("collect_now_requested", requestedAttrs...)

		queued := deps.Collector.CollectNow(collector.CollectRequest{
			Targets:   targetFilter,
			RequestID: requestID,
		})
		if !queued {
			safety.AuditLog("collect_now_dropped",
				"actor", "local_operator",
				"request_id", requestID,
				"reason_category", "previous_request_pending",
			)
		}

		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":           "collection triggered",
			"request_id":       requestID,
			"accepted_targets": responseTargets,
		})
	}
}

// requestedTargetsAuditValue returns either the explicit narrowing
// list or the literal string "all_enabled" so the audit log is
// unambiguous when the caller omitted `targets`. Keeps the audit
// attribute value bounded — R078 forbids unbounded label content.
func requestedTargetsAuditValue(targetFilter, allEnabled []string) any {
	if targetFilter != nil {
		return targetFilter
	}
	return "all_enabled"
}

// newRequestID generates a ULID for the audit-correlation field
// when the caller didn't supply request_id. ULIDs are time-ordered,
// 26 ASCII chars, and naturally satisfy requestIDPattern.
func newRequestID() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}

func handleExport(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		opts := export.Options{
			Since: r.URL.Query().Get("since"),
			Until: r.URL.Query().Get("until"),
		}

		if tid := r.URL.Query().Get("target_id"); tid != "" {
			id, err := strconv.ParseInt(tid, 10, 64)
			if err != nil {
				safety.AuditLog("export_requested",
					"source_ip", remoteIP(r),
					"target_id", tid,
					"since", opts.Since,
					"until", opts.Until,
				)
				safety.AuditLog("export_completed",
					"status", "failed",
					"duration_ms", time.Since(start).Milliseconds(),
					"size_bytes", 0,
					"error_category", "invalid_target_id",
				)
				deps.Metrics.RecordExport("failed", time.Since(start).Seconds())
				deps.Metrics.RecordExportFailure("invalid_target_id")
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target_id"})
				return
			}
			opts.TargetID = id
		}

		safety.AuditLog("export_requested",
			"source_ip", remoteIP(r),
			"target_id", opts.TargetID,
			"since", opts.Since,
			"until", opts.Until,
		)

		// Buffer the ZIP fully before writing any response headers. If the
		// export fails midway we want to return a 500 with an error body, not
		// a 200 with a truncated/invalid ZIP that the client cannot
		// distinguish from success.
		var buf bytes.Buffer
		if err := deps.Exporter.WriteTo(&buf, opts); err != nil {
			slog.Error("export failed", "err", err)
			safety.AuditLog("export_completed",
				"status", "failed",
				"duration_ms", time.Since(start).Milliseconds(),
				"size_bytes", 0,
				"error_category", "builder_error",
			)
			deps.Metrics.RecordExport("failed", time.Since(start).Seconds())
			deps.Metrics.RecordExportFailure("builder_error")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "export failed"})
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=arq-export-%s.zip",
			time.Now().UTC().Format("20060102-150405")))
		w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
		if _, err := w.Write(buf.Bytes()); err != nil {
			slog.Error("export write failed", "err", err)
			safety.AuditLog("export_completed",
				"status", "failed",
				"duration_ms", time.Since(start).Milliseconds(),
				"size_bytes", buf.Len(),
				"error_category", "write_error",
			)
			deps.Metrics.RecordExport("failed", time.Since(start).Seconds())
			deps.Metrics.RecordExportFailure("write_error")
			return
		}
		safety.AuditLog("export_completed",
			"status", "success",
			"duration_ms", time.Since(start).Milliseconds(),
			"size_bytes", buf.Len(),
		)
		deps.Metrics.RecordExport("success", time.Since(start).Seconds())
	}
}

// --- Middleware ---

// tokenRateLimiter tracks invalid bearer token attempts per IP.
type tokenRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*tokenAttempt
}

type tokenAttempt struct {
	failures    int
	lastAttempt time.Time
}

func newTokenRateLimiter() *tokenRateLimiter {
	return &tokenRateLimiter{attempts: make(map[string]*tokenAttempt)}
}

const (
	tokenMaxFailures   = 5
	tokenLockoutWindow = 5 * time.Minute
)

func (l *tokenRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.attempts[ip]
	if !ok {
		return true
	}
	if time.Since(a.lastAttempt) > tokenLockoutWindow {
		delete(l.attempts, ip)
		return true
	}
	return a.failures < tokenMaxFailures
}

func (l *tokenRateLimiter) recordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	a, ok := l.attempts[ip]
	if !ok {
		a = &tokenAttempt{}
		l.attempts[ip] = a
	}
	a.failures++
	a.lastAttempt = now

	// Opportunistic prune: drop entries older than the lockout window.
	// Bounds map growth in long-lived processes where attackers come
	// from many short-lived IPs that never retry.
	cutoff := now.Add(-tokenLockoutWindow)
	for otherIP, other := range l.attempts {
		if other.lastAttempt.Before(cutoff) {
			delete(l.attempts, otherIP)
		}
	}
}

// recordSuccess clears any prior failures for the IP. Called when a
// request authenticates successfully so a near-miss IP does not stay
// near the lockout threshold indefinitely.
func (l *tokenRateLimiter) recordSuccess(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

// recoveryMiddleware catches panics and returns 500.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "panic", rec, "path", r.URL.Path)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs each request.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate request ID.
		b := make([]byte, 8)
		rand.Read(b)
		reqID := hex.EncodeToString(b)
		w.Header().Set("X-Request-ID", reqID)

		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"duration", time.Since(start).String(),
			"request_id", reqID,
		)
	})
}

// tokenAuthMiddleware checks for a valid Bearer token on all requests except /health.
// It also rate-limits invalid token attempts per remote IP.
func tokenAuthMiddleware(token string, limiter *tokenRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for health endpoint (loopback health checks).
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			ip := remoteIP(r)

			if !limiter.allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{"error": "too many invalid token attempts"})
				return
			}

			auth := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(auth, prefix) {
				limiter.recordFailure(ip)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid Authorization header"})
				return
			}

			provided := auth[len(prefix):]
			if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
				limiter.recordFailure(ip)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid API token"})
				return
			}

			// Successful authentication clears any prior failure count for this IP.
			limiter.recordSuccess(ip)
			next.ServeHTTP(w, r)
		})
	}
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
